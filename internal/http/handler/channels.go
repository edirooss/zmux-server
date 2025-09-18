package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/edirooss/zmux-server/internal/domain/channel"
	"github.com/edirooss/zmux-server/internal/domain/channel/views"
	"github.com/edirooss/zmux-server/internal/domain/principal"
	"github.com/edirooss/zmux-server/internal/http/dto"
	"github.com/edirooss/zmux-server/internal/repo"
	"github.com/edirooss/zmux-server/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ChannelsHandler provides RESTful HTTP handlers for Channel resources.
//
// Supported operations:
//   - GET    /channels       → List all channels
//   - POST   /channels       → Create a new channel
//   - GET    /channels/{id}  → Retrieve a channel by ID
//   - PUT    /channels/{id}  → Replace an existing channel (full update)
//   - PATCH  /channels/{id}  → Modify an existing channel (partial update)
//   - DELETE /channels/{id}  → Remove a channel
//
// Notes:
//   - Standard REST semantics (RFC 9110, RFC 5789).
type ChannelsHandler struct {
	log          *zap.Logger
	authsvc      *service.AuthService
	svc          *service.ChannelService
	summarySvc   *service.SummaryService
	b2bClntChnls *repo.B2BClntChnlsRepo
}

// NewChannelsHandler constructs a ChannelsHandler instance.
func NewChannelsHandler(log *zap.Logger, authsvc *service.AuthService, b2bClntChnls *repo.B2BClntChnlsRepo) (*ChannelsHandler, error) {
	// Service for channel CRUD
	channelService, err := service.NewChannelService(log)
	if err != nil {
		return nil, fmt.Errorf("new channel service: %w", err)
	}

	// Service for generating channel summaries
	summarySvc := service.NewSummaryService(
		log,
		service.SummaryOptions{
			TTL:            1000 * time.Millisecond, // tune as needed
			RefreshTimeout: 500 * time.Millisecond,
		},
	)

	return &ChannelsHandler{
		log:          log.Named("channels"),
		authsvc:      authsvc,
		svc:          channelService,
		summarySvc:   summarySvc,
		b2bClntChnls: b2bClntChnls,
	}, nil
}

// GetChannelList handles GET /channels.
//
// Behavior:
//   - If no ID filters are provided, returns all available channels per principal.
//   - If ID filters are provided (?id=... repeated or ?ids=comma,separated),
//     returns only those channels (intersected with principal's visibility for B2B).
//   - Adds `X-Total-Count` header.
//
// Status Codes:
//   - 200 OK  → JSON array of channels
//   - 500 Internal Server Error
func (h *ChannelsHandler) GetChannelList(c *gin.Context) {
	p := h.authsvc.WhoAmI(c) // extract principal (already set by other middleware)

	requestedIDs, err := collectRequestedIDs(c)
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	chs, total, err := h.getChannelListByPrincipal(c.Request.Context(), p, requestedIDs)
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	c.Header("X-Total-Count", strconv.Itoa(total)) // RA needs this
	c.JSON(http.StatusOK, chs)
}

func collectRequestedIDs(c *gin.Context) ([]int64, error) {
	ids := make([]string, 0, 4)

	// Repeated: ?id=x&id=y
	if arr := c.QueryArray("id"); len(arr) > 0 {
		ids = append(ids, arr...)
	}
	// Comma-separated: ?ids=x,y,z
	if csv := c.Query("ids"); csv != "" {
		for _, s := range strings.Split(csv, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				ids = append(ids, s)
			}
		}
	}

	// Dedupe while preserving order and cast to int64
	seen := make(map[string]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}

		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid id %q: %w", id, err)
		}
		if n <= 0 {
			return nil, fmt.Errorf("id must be > 0, got %d", n)
		}
		out = append(out, n)
	}

	return out, nil
}

func (h *ChannelsHandler) getChannelListByPrincipal(ctx context.Context, p *principal.Principal, requestedIDs []int64,
) (interface{}, int, error) {
	if p == nil {
		return nil, 0, fmt.Errorf("nil principal")
	}

	switch p.Kind {
	case principal.Admin:
		// Admin can either fetch all or a filtered subset
		if len(requestedIDs) > 0 {
			chs, err := h.svc.ListChannelsByID(ctx, requestedIDs)
			if err != nil {
				return nil, 0, fmt.Errorf("list channels by id: %w", err)
			}

			adminView := make([]*views.AdminZmuxChannel, len(chs))
			for i := range chs {
				adminView[i] = chs[i].AdminView()
			}
			return adminView, len(adminView), nil
		}

		chs, err := h.svc.ListChannels(ctx)
		if err != nil {
			return nil, 0, fmt.Errorf("list channels: %w", err)
		}
		adminView := make([]*views.AdminZmuxChannel, len(chs))
		for i := range chs {
			adminView[i] = chs[i].AdminView()
		}
		return adminView, len(adminView), nil

	case principal.B2BClient:
		// Get allowed IDs
		allowedIDs, err := h.b2bClntChnls.GetAllMap(ctx, p.ID)
		if err != nil {
			return nil, 0, fmt.Errorf("get b2b channels ids: %w", err)
		}

		var toFetch []int64
		if len(requestedIDs) > 0 {
			// Intersect requestedIDs ∩ allowedIDs
			for _, id := range requestedIDs {
				if _, ok := allowedIDs[id]; ok {
					toFetch = append(toFetch, id)
				}
			}
			if len(toFetch) == 0 {
				// Nothing permitted from the requested subset
				return []*views.B2BClientZmuxChannel{}, 0, nil
			}
		} else {
			// No filter → fetch all allowed
			for allowedID := range allowedIDs {
				toFetch = append(toFetch, allowedID)
			}
		}

		chs, err := h.svc.ListChannelsByID(ctx, toFetch)
		if err != nil {
			return nil, 0, fmt.Errorf("list channels by id: %w", err)
		}
		b2bClientView := make([]*views.B2BClientZmuxChannel, len(chs))
		for i := range chs {
			b2bClientView[i] = chs[i].B2BClientView()
		}
		return b2bClientView, len(b2bClientView), nil
	}

	return nil, 0, fmt.Errorf("unsupported principal")
}

// CreateChannel handles POST /channels.
//
// Behavior:
//   - Validates request body.
//   - Creates a new channel with defaults applied.
//   - Responds with resource location in `Location` header.
//
// Status Codes:
//   - 201 Created → JSON of created channel
//   - 400 Bad Request → Invalid JSON or schema
//   - 422 Unprocessable Entity → Validation failed
//   - 500 Internal Server Error
func (h *ChannelsHandler) CreateChannel(c *gin.Context) {
	var req dto.ChannelCreate
	if err := bind(c.Request, &req); err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	ch, err := req.ToChannel()
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	if err := ch.Validate(); err != nil {
		c.Error(err)
		c.JSON(http.StatusUnprocessableEntity, gin.H{"message": err.Error()})
		return
	}

	if err := h.svc.CreateChannel(c.Request.Context(), ch); err != nil {
		c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.Header("Location", fmt.Sprintf("/api/channels/%d", ch.ID))
	c.JSON(http.StatusCreated, ch)
}

// GetChannel handles GET /channels/{id}.
//
// Behavior:
//   - Retrieves a single channel by ID.
//   - Returns 404 if channel does not exist.
//
// Status Codes:
//   - 200 OK → JSON of channel
//   - 400 Bad Request → Invalid ID format
//   - 404 Not Found → Channel not found
//   - 500 Internal Server Error
func (h *ChannelsHandler) GetChannel(c *gin.Context) {
	p := h.authsvc.WhoAmI(c)                         // extract principal (already set by other middleware)
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64) // extract :id (already validated by middleware)

	ch, err := h.getChannelByPrincipal(c.Request.Context(), p, id)
	if err != nil {
		c.Error(err)
		if errors.Is(err, repo.ErrChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": repo.ErrChannelNotFound.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, ch)
}

func (h *ChannelsHandler) getChannelByPrincipal(ctx context.Context, p *principal.Principal, id int64) (interface{}, error) {
	if p == nil {
		return nil, fmt.Errorf("nil principal")
	}

	ch, err := h.svc.GetChannel(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get channel: %w", err)
	}

	switch p.Kind {

	case principal.Admin:
		return ch.AdminView(), nil

	case principal.B2BClient:
		return ch.B2BClientView(), nil
	}

	return nil, fmt.Errorf("unsupported principal")
}

// ModifyChannel handles PATCH /channels/{id}.
//
// Behavior:
//   - Partially updates a channel (merge-patch style).
//   - Only provided fields are updated.
//
// Status Codes:
//   - 204 No Content → Success
//   - 400 Bad Request → Invalid ID or payload
//   - 404 Not Found → Channel not found
//   - 422 Unprocessable Entity → Validation failed
//   - 500 Internal Server Error
func (h *ChannelsHandler) ModifyChannel(c *gin.Context) {
	p := h.authsvc.WhoAmI(c)                         // extract principal (already set by other middleware)
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64) // extract :id (already validated by middleware)

	// Load current
	ch, err := h.svc.GetChannel(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		if errors.Is(err, repo.ErrChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": repo.ErrChannelNotFound.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	var req dto.ChannelModify
	if err := bind(c.Request, &req); err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	code, err := h.patchAndUpdate(c.Request.Context(), &req, ch, p.Kind)
	if err != nil {
		c.Error(err)
		c.JSON(code, gin.H{"message": err.Error()})
		return
	}

	c.Status(code)
}

func (h *ChannelsHandler) patchAndUpdate(ctx context.Context, req *dto.ChannelModify, ch *channel.ZmuxChannel, pKind principal.PrincipalKind) (int, error) {
	// Apply patch
	if err := req.MergePatch(ch, pKind); err != nil {
		return http.StatusBadRequest, err
	}

	// Validate
	if err := ch.Validate(); err != nil {
		return http.StatusUnprocessableEntity, err
	}

	// Persist
	if err := h.svc.UpdateChannel(ctx, ch); err != nil {
		if errors.Is(err, service.ErrLocked) {
			return http.StatusLocked, err
		}
		if errors.Is(err, repo.ErrChannelNotFound) {
			return http.StatusNotFound, err
		}
		return http.StatusInternalServerError, err
	}

	return http.StatusNoContent, nil
}

// ReplaceChannel handles PUT /channels/{id}.
//
// Behavior:
//   - Replaces an existing channel with a full payload.
//
// Status Codes:
//   - 200 OK → JSON of updated channel
//   - 400 Bad Request → Invalid ID or payload
//   - 404 Not Found → Channel not found
//   - 422 Unprocessable Entity → Validation failed
//   - 500 Internal Server Error
func (h *ChannelsHandler) ReplaceChannel(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64) // extract :id (already validated by middleware)

	exists, err := h.svc.ChannelExists(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	if !exists {
		c.Status(http.StatusNotFound)
		return
	}

	var req dto.ChannelReplace
	if err := bind(c.Request, &req); err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	// Replace obj
	ch, err := req.ToChannel(id)
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	if err := ch.Validate(); err != nil {
		c.Error(err)
		c.JSON(http.StatusUnprocessableEntity, gin.H{"message": err.Error()})
		return
	}

	if err := h.svc.UpdateChannel(c.Request.Context(), ch); err != nil {
		c.Error(err)
		if errors.Is(err, service.ErrLocked) {
			c.JSON(http.StatusLocked, gin.H{"message": service.ErrLocked.Error()})
			return
		}
		if errors.Is(err, repo.ErrChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": repo.ErrChannelNotFound.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, ch)
}

// DeleteChannel handles DELETE /channels/{id}.
//
// Behavior:
//   - Removes a channel by ID.
//
// Status Codes:
//   - 200 OK → JSON { "id": deletedID }
//   - 400 Bad Request → Invalid ID
//   - 404 Not Found → Channel not found
//   - 500 Internal Server Error
func (h *ChannelsHandler) DeleteChannel(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64) // extract :id (already validated by middleware)

	if err := h.svc.DeleteChannel(c.Request.Context(), id); err != nil {
		c.Error(err)
		if errors.Is(err, service.ErrLocked) {
			c.JSON(http.StatusLocked, gin.H{"message": service.ErrLocked.Error()})
			return
		}
		if errors.Is(err, repo.ErrChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": repo.ErrChannelNotFound.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	// RA-friendly response
	c.JSON(http.StatusOK, gin.H{"id": id})
}

type itemResult struct {
	ID     int64  `json:"id"`
	Status int    `json:"status"`
	Error  string `json:"error,omitempty"`
}

func (h *ChannelsHandler) DeleteChannels(c *gin.Context) {
	requestedIDs, err := collectRequestedIDs(c)
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	results := make([]itemResult, 0, len(requestedIDs))
	deleted := make([]int64, 0, len(requestedIDs))
	failed := make([]int64, 0, len(requestedIDs))

	// requestedIDs appears to be a set (map[string]struct{}). If it's a slice, change `range` accordingly.
	for _, id := range requestedIDs {
		if err := h.svc.DeleteChannel(c.Request.Context(), id); err != nil {
			c.Error(err)

			status := http.StatusInternalServerError
			msg := err.Error()

			switch {
			case errors.Is(err, service.ErrLocked):
				status = http.StatusLocked // 423
				msg = service.ErrLocked.Error()
			case errors.Is(err, repo.ErrChannelNotFound):
				status = http.StatusNotFound // 404
				msg = repo.ErrChannelNotFound.Error()
			}

			results = append(results, itemResult{
				ID:     id,
				Status: status,
				Error:  msg,
			})
			failed = append(failed, id)
			continue
		}

		results = append(results, itemResult{
			ID:     id,
			Status: http.StatusOK, // 200
		})
		deleted = append(deleted, id)
	}

	// Decide top-level HTTP status:
	// - 200 OK when all succeeded
	// - 207 Multi-Status when mixed outcomes (some failures)
	status := http.StatusOK
	if len(failed) > 0 {
		status = http.StatusMultiStatus
	}

	c.JSON(status, gin.H{
		"count": gin.H{
			"attempted": len(requestedIDs),
			"deleted":   len(deleted),
			"failed":    len(failed),
		},
		"data": gin.H{
			"deleted": deleted,
			"failed":  failed,
		},
		"results": results,
	})
}

func (h *ChannelsHandler) ModifyChannels(c *gin.Context) {
	p := h.authsvc.WhoAmI(c) // extract principal (already set by other middleware)
	requestedIDs, err := collectRequestedIDs(c)
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	var req dto.ChannelModify
	if err := bind(c.Request, &req); err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	results := make([]itemResult, 0, len(requestedIDs))
	updated := make([]int64, 0, len(requestedIDs))
	failed := make([]int64, 0, len(requestedIDs))

	for _, id := range requestedIDs {
		// Load current
		ch, err := h.svc.GetChannel(c.Request.Context(), id)
		if err != nil {
			c.Error(err)

			status := http.StatusInternalServerError
			msg := err.Error()

			if errors.Is(err, repo.ErrChannelNotFound) {
				status = http.StatusNotFound
				msg = repo.ErrChannelNotFound.Error()
			}

			results = append(results, itemResult{
				ID:     id,
				Status: status,
				Error:  msg,
			})
			failed = append(failed, id)
			continue
		}

		code, err := h.patchAndUpdate(c.Request.Context(), &req, ch, p.Kind)
		if err != nil {
			c.Error(err)
			status := code
			msg := err.Error()
			results = append(results, itemResult{
				ID:     id,
				Status: status,
				Error:  msg,
			})
			failed = append(failed, id)
			continue
		}

		results = append(results, itemResult{
			ID:     id,
			Status: http.StatusOK, // 200
		})
		updated = append(updated, id)
	}

	// Decide top-level HTTP status:
	// - 200 OK when all succeeded
	// - 207 Multi-Status when mixed outcomes (some failures)
	status := http.StatusOK
	if len(failed) > 0 {
		status = http.StatusMultiStatus
	}

	c.JSON(status, gin.H{
		"count": gin.H{
			"attempted": len(requestedIDs),
			"updated":   len(updated),
			"failed":    len(failed),
		},
		"data": gin.H{
			"updated": updated,
			"failed":  failed,
		},
		"results": results,
	})
}

//
// ----- Helpers -----

func bind(req *http.Request, obj any) error {
	if req == nil || req.Body == nil {
		return errors.New("invalid request")
	}
	return decodeJSON(req.Body, obj)
}

func decodeJSON(r io.Reader, obj any) error {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(obj); err != nil {
		return err
	}
	return nil
}

// ------ Summary -----
func (h *ChannelsHandler) Summary(c *gin.Context) {
	// Optional query to bypass cache for admin/diagnostics: ?force=1
	force := c.Query("force") == "1"

	var (
		res service.SummaryResult
		err error
	)
	if force {
		// Force a refresh by temporarily setting TTL=0 via a context trick:
		// Simply call summarySvc.Get with expired cache by invalidating before.
		// Safer: expose a public Invalidate(). We'll do that:
		h.summarySvc.Invalidate()
	}

	res, err = h.summarySvc.Get(c.Request.Context())
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	// Friendly cache headers for debugging/observability
	c.Header("X-Cache", map[bool]string{true: "HIT", false: "MISS"}[res.CacheHit])
	c.Header("X-Summary-Generated-At", strconv.FormatInt(res.GeneratedAt.UnixMilli(), 10))
	c.Header("X-Total-Count", strconv.Itoa(len(res.Data)))

	c.JSON(http.StatusOK, res.Data)
}

// ---- Channel Status List -----
// Prototype/demo -- quick win based on Summary.
func (h *ChannelsHandler) Status(c *gin.Context) {
	summaryResult, err := h.summarySvc.Get(c.Request.Context())
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	p := h.authsvc.WhoAmI(c)

	var clntChnlsIDs map[int64]struct{}
	if p.Kind == principal.B2BClient {
		clntChnlsIDs, err = h.b2bClntChnls.GetAllMap(c.Request.Context(), p.ID)
		if err != nil {
			c.Error(err)
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			return
		}
	}

	out := make([]dto.ChannelStatus, 0, len(summaryResult.Data))
	for _, item := range summaryResult.Data {
		channelStatus := dto.ChannelStatus{
			ID:     item.ID,
			Online: item.Status != nil && item.Status.Online,
		}
		switch p.Kind {
		case principal.Admin:
			out = append(out, channelStatus)
		case principal.B2BClient:
			if _, ok := clntChnlsIDs[item.ID]; ok {
				out = append(out, channelStatus)
			}
		}
	}

	// Friendly cache headers for debugging/observability
	c.Header("X-Cache", map[bool]string{true: "HIT", false: "MISS"}[summaryResult.CacheHit])
	c.Header("X-Status-Generated-At", strconv.FormatInt(summaryResult.GeneratedAt.UnixMilli(), 10))
	c.Header("X-Total-Count", strconv.Itoa(len(out)))

	c.JSON(http.StatusOK, out)
}

// Quota
// Prototype/demo
func (h *ChannelsHandler) Quota(c *gin.Context) {
	p := h.authsvc.WhoAmI(c)
	chIDs, err := h.b2bClntChnls.GetAll(c.Request.Context(), p.ID)
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	chs, err := h.svc.ListChannelsByID(c.Request.Context(), chIDs)
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	// Compute used enabled
	used := 0
	for _, ch := range chs {
		if ch.Enabled {
			used++
		}
	}

	c.JSON(http.StatusOK, struct {
		Limit     *int `json:"limit"`
		Used      int  `json:"used"`
		Remaining *int `json:"remaining"`
	}{nil, used, nil})
}
