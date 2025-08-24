package channelshndlr

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"

	"github.com/edirooss/zmux-server/pkg/models/channelmodel"
	"github.com/edirooss/zmux-server/redis"
	"github.com/gin-gonic/gin"

	jsonpatch "github.com/evanphx/json-patch/v5"
)

func (h *ChannelsHandler) PartialUpdateChannel(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid id"})
		return
	}

	// 1) Enforce RFC 7396 media type
	mt, _, _ := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if mt != "application/merge-patch+json" {
		c.JSON(http.StatusUnsupportedMediaType, gin.H{
			"message": "only application/merge-patch+json is supported for PATCH",
		})
		return
	}

	// 2) Read body safely
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20)) // 1MB cap
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": "unable to read request body"})
		return
	}
	if len(body) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "empty patch body"})
		return
	}
	if !json.Valid(body) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "malformed JSON"})
		return
	}

	// 3) Validate patch shape/types against ChannelPatchRequest (additionalProperties: false etc.)
	if err := validateChannelPatchDocument(body); err != nil {
		// 400 for shape/type/unknown-field/unauthorized errors
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	// 4) Load current resource
	current, err := h.svc.GetChannel(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		if errors.Is(err, redis.ErrChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": redis.ErrChannelNotFound.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	// 5) Apply JSON Merge Patch
	origJSON, err := json.Marshal(current)
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to serialize current resource"})
		return
	}
	patchedJSON, err := jsonpatch.MergePatch(origJSON, body)
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid merge patch"})
		return
	}

	// 6) Unmarshal into domain model
	var candidate channelmodel.ZmuxChannel
	if err := json.Unmarshal(patchedJSON, &candidate); err != nil {
		c.Error(err)
		// This typically hits when a field was set to null but the domain type is non-nullable.
		c.JSON(http.StatusBadRequest, gin.H{"message": "patched resource is invalid JSON for channel model"})
		return
	}

	// 7) Field-level constraints that the spec marks as 400/422
	//    We only enforce on fields *present in the patch* (classic PATCH behavior).
	if err := semanticValidatePatchedFields(body); err != nil {
		var se *semErr
		if errors.As(err, &se) && se.status != 0 {
			c.JSON(se.status, gin.H{"message": se.Error()})
			return
		}
		// fallback
		c.JSON(http.StatusUnprocessableEntity, gin.H{"message": err.Error()})
		return
	}

	// 8) Cross-field/domain constraints (422 on failure)
	if err := candidate.Validate(); err != nil {
		c.Error(err)
		c.JSON(http.StatusUnprocessableEntity, gin.H{"message": err.Error()})
		return
	}

	// 9) No-op detection → still 204
	if bytes.Equal(origJSON, patchedJSON) {
		c.Status(http.StatusNoContent)
		return
	}

	// 10) Persist
	if err := h.svc.UpdateChannel(c.Request.Context(), &candidate); err != nil {
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

var schemeRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.\-]*:`)

type semErr struct {
	status  int
	message string
}

func (e *semErr) Error() string { return e.message }

// Validates shape/types/unknown props for ChannelPatchRequest.
// 400 for:
// - Unknown fields
// - Wrong JSON types
// - Null where not allowed (e.g., enabled, input)
func validateChannelPatchDocument(raw []byte) error {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}

	allowedTop := map[string]struct{}{"name": {}, "input": {}, "enabled": {}}
	for k := range m {
		if _, ok := allowedTop[k]; !ok {
			return fmt.Errorf("unknown field: %q", k)
		}
	}

	// name: string|null
	if v, ok := m["name"]; ok {
		if v != nil {
			if _, ok := v.(string); !ok {
				return errors.New("name must be a string or null")
			}
		}
	}

	// enabled: boolean ONLY (no null)
	if v, ok := m["enabled"]; ok {
		if v == nil {
			return errors.New("enabled cannot be null")
		}
		if _, ok := v.(bool); !ok {
			return errors.New("enabled must be a boolean")
		}
	}

	// input: object ONLY (no null), additionalProperties: false; only "url"
	if v, ok := m["input"]; ok {
		if v == nil {
			return errors.New("input cannot be null")
		}
		obj, ok := v.(map[string]any)
		if !ok {
			return errors.New("input must be an object")
		}
		allowedInput := map[string]struct{}{"url": {}}
		for k := range obj {
			if _, ok := allowedInput[k]; !ok {
				return fmt.Errorf("unknown field: %q under input", k)
			}
		}
		// input.url: string|null
		if uv, ok := obj["url"]; ok {
			if uv != nil {
				if _, ok := uv.(string); !ok {
					return errors.New("input.url must be a string or null")
				}
			}
		}
	}

	return nil
}

// Enforces property-level constraints from the schema on fields that are PRESENT in the patch.
// - URI syntax issues → 400
// - min/max length → 422
func semanticValidatePatchedFields(raw []byte) error {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}

	// name constraints when present and non-null
	if v, ok := m["name"]; ok && v != nil {
		name := v.(string)
		if l := len(name); l < 1 || l > 100 {
			return &semErr{status: http.StatusUnprocessableEntity, message: "name length must be between 1 and 100"}
		}
	}

	// input.url constraints when present (null allowed = clear)
	if iv, ok := m["input"]; ok && iv != nil {
		obj := iv.(map[string]any)
		if uv, ok := obj["url"]; ok && uv != nil {
			u := uv.(string)

			// length → 422
			if len(u) > 2048 {
				return &semErr{status: http.StatusUnprocessableEntity, message: "input.url exceeds max length 2048"}
			}
			// must look like scheme: → 400
			if !schemeRe.MatchString(u) {
				return &semErr{status: http.StatusBadRequest, message: "input.url must include an absolute URI scheme"}
			}
			// parse absolute → 400
			pu, err := url.Parse(u)
			if err != nil || !pu.IsAbs() {
				return &semErr{status: http.StatusBadRequest, message: "input.url must be an absolute URI"}
			}
			// More protocol-specific validation (RTSP, rtsps, etc.) is delegated to domain validation.
		}
	}

	return nil
}
