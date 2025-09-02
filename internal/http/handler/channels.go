package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/edirooss/zmux-server/internal/domain/channel"
	"github.com/edirooss/zmux-server/internal/domain/principal"
	"github.com/edirooss/zmux-server/internal/env"
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
	log        *zap.Logger
	authsvc    *service.AuthService
	svc        *service.ChannelService
	summarySvc *service.SummaryService
}

// NewChannelsHandler constructs a ChannelsHandler instance.
func NewChannelsHandler(log *zap.Logger, authsvc *service.AuthService) (*ChannelsHandler, error) {
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
		log:        log.Named("channels"),
		authsvc:    authsvc,
		svc:        channelService,
		summarySvc: summarySvc,
	}, nil
}

// GetChannelList handles GET /channels.
//
// Behavior:
//   - Returns all available channels.
//   - Adds `X-Total-Count` header.
//
// Status Codes:
//   - 200 OK  → JSON array of channels
//   - 500 Internal Server Error
func (h *ChannelsHandler) GetChannelList(c *gin.Context) {
	p := h.authsvc.WhoAmI(c)

	var chs []*channel.ZmuxChannel
	var err error
	switch p.Kind {
	case principal.Admin:
		chs, err = h.svc.ListChannels(c.Request.Context())
	case principal.B2BClient:
		chs, err = h.svc.GetMany(c.Request.Context(), env.B2BClientChannelIDsIndex.ChannelIDs(p.ID))
	}

	if err != nil {
		c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	c.Header("X-Total-Count", strconv.Itoa(len(chs))) // RA needs this
	c.JSON(http.StatusOK, chs)
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
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64) // extract :id (already validated by middleware)

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

	c.JSON(http.StatusOK, ch)
}

// ModifyChannel handles PATCH /channels/{id}.
//
// Behavior:
//   - Partially updates a channel (merge-patch style).
//   - Only provided fields are modified.
//
// Status Codes:
//   - 204 No Content → Success
//   - 400 Bad Request → Invalid ID or payload
//   - 404 Not Found → Channel not found
//   - 422 Unprocessable Entity → Validation failed
//   - 500 Internal Server Error
func (h *ChannelsHandler) ModifyChannel(c *gin.Context) {
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

	// Patch obj
	if err := req.MergePatch(ch); err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	if err := ch.Validate(); err != nil {
		c.Error(err)
		c.JSON(http.StatusUnprocessableEntity, gin.H{"message": err.Error()})
		return
	}

	// Persist
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

	c.Status(http.StatusNoContent)
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

	data := make([]dto.ChannelStatus, 0, len(summaryResult.Data))
	for _, item := range summaryResult.Data {
		if p.Kind == principal.Admin || (p.Kind == principal.B2BClient && env.B2BClientChannelIDsIndex.Has(p.ID, item.ID)) {
			data = append(data, dto.ChannelStatus{
				ID:     item.ID,
				Online: item.Status != nil && item.Status.Liveness == "Live",
			})
		}
	}

	// Friendly cache headers for debugging/observability
	c.Header("X-Cache", map[bool]string{true: "HIT", false: "MISS"}[summaryResult.CacheHit])
	c.Header("X-Status-Generated-At", strconv.FormatInt(summaryResult.GeneratedAt.UnixMilli(), 10))
	c.Header("X-Total-Count", strconv.Itoa(len(data)))

	c.JSON(http.StatusOK, data)
}

// Quota
// Prototype/demo
func (h *ChannelsHandler) Quota(c *gin.Context) {
	p := h.authsvc.WhoAmI(c)

	chs, err := h.svc.GetMany(c.Request.Context(), env.B2BClientChannelIDsIndex.ChannelIDs(p.ID))

	if err != nil {
		c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

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
