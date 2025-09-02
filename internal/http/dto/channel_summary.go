package dto

import (
	"encoding/json"

	"github.com/edirooss/zmux-server/internal/domain/channel"
	"github.com/edirooss/zmux-server/internal/repo"
)

// ChannelSummary is the API model for GET /api/channels/summary.
// We embed ZmuxChannel so its fields are flattened (id, name, etc.) and
// add monitoring fields conditionally.
//   - status is present only if channel.Enabled == true AND status key exists.
//   - ifmt/metrics are present only if status.liveness == "Live" and keys exist.
type ChannelSummary struct {
	channel.ZmuxChannel
	Status  *repo.RemuxStatus `json:"status,omitempty"`
	Ifmt    json.RawMessage   `json:"ifmt,omitempty"`
	Metrics json.RawMessage   `json:"metrics,omitempty"`
}
