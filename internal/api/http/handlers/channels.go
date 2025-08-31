package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/edirooss/zmux-server/internal/api/http/dto"
	"github.com/edirooss/zmux-server/internal/redis"
	"github.com/edirooss/zmux-server/internal/services"
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
	svc        *services.ChannelService
	summarySvc *services.SummaryService
}

// NewChannelsHandler constructs a ChannelsHandler instance.
func NewChannelsHandler(log *zap.Logger) (*ChannelsHandler, error) {
	// Service for channel CRUD
	channelService, err := services.NewChannelService(log)
	if err != nil {
		return nil, fmt.Errorf("new channel service: %w", err)
	}

	// Service for generating channel summaries
	summarySvc := services.NewSummaryService(
		log,
		services.SummaryOptions{
			TTL:            1000 * time.Millisecond, // tune as needed
			RefreshTimeout: 500 * time.Millisecond,
		},
	)

	return &ChannelsHandler{
		log:        log.Named("channels"),
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
	chs, err := h.svc.ListChannels(c.Request.Context())
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
	var req dto.CreateChannel
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
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid id"})
		return
	}

	ch, err := h.svc.GetChannel(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		if errors.Is(err, redis.ErrChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": redis.ErrChannelNotFound.Error()})
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
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid id"})
		return
	}

	// Load current
	ch, err := h.svc.GetChannel(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		if errors.Is(err, redis.ErrChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": redis.ErrChannelNotFound.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	var req dto.ModifyChannel
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
		if errors.Is(err, redis.ErrChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": redis.ErrChannelNotFound.Error()})
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
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid id"})
		return
	}

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

	var req dto.ReplaceChannel
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
		if errors.Is(err, redis.ErrChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": redis.ErrChannelNotFound.Error()})
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
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid id"})
		return
	}

	if err := h.svc.DeleteChannel(c.Request.Context(), id); err != nil {
		c.Error(err)
		if errors.Is(err, redis.ErrChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": redis.ErrChannelNotFound.Error()})
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
		res services.SummaryResult
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
