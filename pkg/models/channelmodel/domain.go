// File: domain.go
// Domain holds core business types and default values.
package channelmodel

import (
	"bytes"
	"encoding/json"
	"net/netip"
	"net/url"
)

// ===== Enumerations / constrained values =====

type AVIOFlag string

const (
	AVIOFlagDirect AVIOFlag = "direct"
)

type FFlag string

const (
	FFlagNoBuffer FFlag = "nobuffer"
)

type RTSPTransport string

const (
	RTSPTransportTCP          RTSPTransport = "tcp"
	RTSPTransportUDP          RTSPTransport = "udp"
	RTSPTransportUDPMulticast RTSPTransport = "udp_multicast"
)

func (rt *RTSPTransport) String() string {
	if rt == nil {
		return ""
	}
	return string(*rt)
}

// ===== Defaults (single source of truth for service layer) =====

const (
	// Input defaults
	DefaultProbesize       uint = 5_000_000
	DefaultAnalyzeduration uint = 0
	DefaultMaxDelay        int  = -1
	DefaultTimeout         uint = 3_000_000

	// Output defaults
	DefaultPktSize uint = 1_316

	// Systemd / process defaults
	DefaultEnabled    bool = false
	DefaultRestartSec uint = 3
)

// ===== JSONURL: wrapper to round-trip URLs as strings =====

type JSONURL struct{ *url.URL }

func (j *JSONURL) MarshalJSON() ([]byte, error) {
	if j == nil || j.URL == nil {
		return []byte("null"), nil
	}
	return json.Marshal(j.String())
}

func (j *JSONURL) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if bytes.Equal(b, []byte("null")) {
		j.URL = nil
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	u, err := url.Parse(s)
	if err != nil {
		return err
	}
	j.URL = u
	return nil
}

// ===== Domain model (with JSON tags; no `omitempty` so nulls are emitted) =====

// ZmuxChannel is the canonical domain entity.
// Nullable fields are pointers. URLs/IPs are strongly typed.
type ZmuxChannel struct {
	ID         int64   `json:"id"`
	Name       *string `json:"name"` // nullable
	Input      Input   `json:"input"`
	Output     Output  `json:"output"`
	Enabled    bool    `json:"enabled"`
	RestartSec uint    `json:"restart_sec"`
}

// Input captures input-side ffmpeg/IO settings.
type Input struct {
	URL             *JSONURL       `json:"url"`             // nullable
	AVIOFlags       []AVIOFlag     `json:"avioflags"`       // default []
	Probesize       uint           `json:"probesize"`       // default 5_000_000
	Analyzeduration uint           `json:"analyzeduration"` // default 0
	FFlags          []FFlag        `json:"fflags"`          // default ["nobuffer"]
	MaxDelay        int            `json:"max_delay"`       // default -1
	Localaddr       *netip.Addr    `json:"localaddr"`       // nullable IPv4
	Timeout         uint           `json:"timeout"`         // default 3_000_000
	RTSPTransport   *RTSPTransport `json:"rtsp_transport"`  // nullable; tcp|udp|udp_multicast
}

// Output captures output-side settings.
type Output struct {
	URL       *JSONURL    `json:"url"`       // nullable
	Localaddr *netip.Addr `json:"localaddr"` // nullable IPv4
	PktSize   uint        `json:"pkt_size"`  // default 1316
	MapVideo  bool        `json:"map_video"` // default true
	MapAudio  bool        `json:"map_audio"` // default true
	MapData   bool        `json:"map_data"`  // default true
}

// NewZmuxChannel returns a channel with all domain defaults applied.
// Service/transport layers can override/merge based on request semantics.
func NewZmuxChannel(id int64) ZmuxChannel {
	return ZmuxChannel{
		ID:   id,
		Name: nil,
		Input: Input{
			URL:             nil,
			AVIOFlags:       make([]AVIOFlag, 0),
			Probesize:       DefaultProbesize,
			Analyzeduration: DefaultAnalyzeduration,
			FFlags:          []FFlag{FFlagNoBuffer},
			MaxDelay:        DefaultMaxDelay,
			Localaddr:       nil,
			Timeout:         DefaultTimeout,
			RTSPTransport:   nil,
		},
		Output: Output{
			URL:       nil,
			Localaddr: nil,
			PktSize:   DefaultPktSize,
			MapVideo:  true,
			MapAudio:  true,
			MapData:   true,
		},
		Enabled:    DefaultEnabled,
		RestartSec: DefaultRestartSec,
	}
}
