package models

import (
	"encoding/json"

	"github.com/edirooss/zmux-server/internal/domain/channel"
)

// RemuxStatus mirrors the JSON stored at remux:<id>:status
// Example stored value (string):
//
//	{
//	  "liveness": "Dead" | "Live",
//	  "metadata": "...",
//	  "timestamp": 0
//	}
//
// Keep field names/json tags aligned with stored JSON to avoid re-mapping.
type RemuxStatus struct {
	Liveness  string `json:"liveness"`
	Metadata  string `json:"metadata"`
	Timestamp int64  `json:"timestamp"`
}

// ChannelSummary is the API model for GET /api/channels/summary.
// We embed ZmuxChannel so its fields are flattened (id, name, etc.) and
// add monitoring fields conditionally.
//   - status is present only if channel.Enabled == true AND status key exists.
//   - ifmt/metrics are present only if status.liveness == "Live" and keys exist.
type ChannelSummary struct {
	channel.ZmuxChannel
	Status  *RemuxStatus    `json:"status,omitempty"`
	Ifmt    json.RawMessage `json:"ifmt,omitempty"`
	Metrics json.RawMessage `json:"metrics,omitempty"`
}
