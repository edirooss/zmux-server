package channelmodel

// package dto
// PUT /zmux/channels/{id} — full replacement.
// - All top-level and nested fields are REQUIRED to be present in the JSON.
// - Nullable fields may be explicit null.
// - Non-nullable fields must not be null.
// - After mapping, call z.Validate(OpReplace).

import (
	"fmt"
	"net/netip"
	"net/url"

	"github.com/edirooss/zmux-server/pkg/jsonx"
)

// ===== Replace DTOs =====

type ReplaceZmuxChannelReq struct {
	Name       jsonx.Field[string]        `json:"name"`        // REQUIRED; nullable
	Input      jsonx.Field[ReplaceInput]  `json:"input"`       // REQUIRED; NOT nullable
	Output     jsonx.Field[ReplaceOutput] `json:"output"`      // REQUIRED; NOT nullable
	Enabled    jsonx.Field[bool]          `json:"enabled"`     // REQUIRED; NOT nullable
	RestartSec jsonx.Field[uint]          `json:"restart_sec"` // REQUIRED; NOT nullable
}

type ReplaceInput struct {
	URL             jsonx.Field[string]   `json:"url"`             // REQUIRED; nullable
	AVIOFlags       jsonx.Field[[]string] `json:"avioflags"`       // REQUIRED; NOT nullable
	Probesize       jsonx.Field[uint]     `json:"probesize"`       // REQUIRED; NOT nullable
	Analyzeduration jsonx.Field[uint]     `json:"analyzeduration"` // REQUIRED; NOT nullable
	FFlags          jsonx.Field[[]string] `json:"fflags"`          // REQUIRED; NOT nullable
	MaxDelay        jsonx.Field[int]      `json:"max_delay"`       // REQUIRED; NOT nullable
	Localaddr       jsonx.Field[string]   `json:"localaddr"`       // REQUIRED; nullable (IPv4)
	Timeout         jsonx.Field[uint]     `json:"timeout"`         // REQUIRED; NOT nullable
	RTSPTransport   jsonx.Field[string]   `json:"rtsp_transport"`  // REQUIRED; nullable ("tcp"|"udp"|"udp_mutlicast")
}

type ReplaceOutput struct {
	URL       jsonx.Field[string] `json:"url"`       // REQUIRED; nullable
	Localaddr jsonx.Field[string] `json:"localaddr"` // REQUIRED; nullable (IPv4)
	PktSize   jsonx.Field[uint]   `json:"pkt_size"`  // REQUIRED; NOT nullable
	MapVideo  jsonx.Field[bool]   `json:"map_video"` // REQUIRED; NOT nullable
	MapAudio  jsonx.Field[bool]   `json:"map_audio"` // REQUIRED; NOT nullable
	MapData   jsonx.Field[bool]   `json:"map_data"`  // REQUIRED; NOT nullable
}

// ===== Mapping =====

// ToDomain builds a ZmuxChannel as a full replacement.
// All fields (and nested fields) must be present; nullable fields may be null.
// Call domain validation afterwards: z.Validate(OpReplace).
func (r ReplaceZmuxChannelReq) ToDomain(id int64) (*ZmuxChannel, error) {
	// ---- top-level presence ----
	if !r.Name.IsSet() {
		return nil, fieldMissingErr("name")
	}
	if !r.Input.IsSet() {
		return nil, fieldMissingErr("input")
	}
	if r.Input.IsNull() {
		return nil, fieldNullErr("input")
	}
	if !r.Output.IsSet() {
		return nil, fieldMissingErr("output")
	}
	if r.Output.IsNull() {
		return nil, fieldNullErr("output")
	}
	if !r.Enabled.IsSet() {
		return nil, fieldMissingErr("enabled")
	}
	if r.Enabled.IsNull() {
		return nil, fieldNullErr("enabled")
	}
	if !r.RestartSec.IsSet() {
		return nil, fieldMissingErr("restart_sec")
	}
	if r.RestartSec.IsNull() {
		return nil, fieldNullErr("restart_sec")
	}

	in := *r.Input.Value()
	out := *r.Output.Value()

	// ---- nested presence: input ----
	if !in.URL.IsSet() {
		return nil, fieldMissingErr("input.url")
	}
	if !in.AVIOFlags.IsSet() {
		return nil, fieldMissingErr("input.avioflags")
	}
	if in.AVIOFlags.IsNull() {
		return nil, fieldNullErr("input.avioflags")
	}
	if !in.Probesize.IsSet() {
		return nil, fieldMissingErr("input.probesize")
	}
	if in.Probesize.IsNull() {
		return nil, fieldNullErr("input.probesize")
	}
	if !in.Analyzeduration.IsSet() {
		return nil, fieldMissingErr("input.analyzeduration")
	}
	if in.Analyzeduration.IsNull() {
		return nil, fieldNullErr("input.analyzeduration")
	}
	if !in.FFlags.IsSet() {
		return nil, fieldMissingErr("input.fflags")
	}
	if in.FFlags.IsNull() {
		return nil, fieldNullErr("input.fflags")
	}
	if !in.MaxDelay.IsSet() {
		return nil, fieldMissingErr("input.max_delay")
	}
	if in.MaxDelay.IsNull() {
		return nil, fieldNullErr("input.max_delay")
	}
	if !in.Localaddr.IsSet() {
		return nil, fieldMissingErr("input.localaddr")
	}
	if !in.Timeout.IsSet() {
		return nil, fieldMissingErr("input.timeout")
	}
	if in.Timeout.IsNull() {
		return nil, fieldNullErr("input.timeout")
	}
	if !in.RTSPTransport.IsSet() {
		return nil, fieldMissingErr("input.rtsp_transport")
	}

	// ---- nested presence: output ----
	if !out.URL.IsSet() {
		return nil, fieldMissingErr("output.url")
	}
	if !out.Localaddr.IsSet() {
		return nil, fieldMissingErr("output.localaddr")
	}
	if !out.PktSize.IsSet() {
		return nil, fieldMissingErr("output.pkt_size")
	}
	if out.PktSize.IsNull() {
		return nil, fieldNullErr("output.pkt_size")
	}
	if !out.MapVideo.IsSet() {
		return nil, fieldMissingErr("output.map_video")
	}
	if out.MapVideo.IsNull() {
		return nil, fieldNullErr("output.map_video")
	}
	if !out.MapAudio.IsSet() {
		return nil, fieldMissingErr("output.map_audio")
	}
	if out.MapAudio.IsNull() {
		return nil, fieldNullErr("output.map_audio")
	}
	if !out.MapData.IsSet() {
		return nil, fieldMissingErr("output.map_data")
	}
	if out.MapData.IsNull() {
		return nil, fieldNullErr("output.map_data")
	}

	// ---- build domain (full replacement) ----
	z := NewZmuxChannel(id)

	// name (nullable)
	if r.Name.IsNull() {
		z.Name = nil
	} else if v := *r.Name.Value(); true {
		z.Name = &v
	}

	// input.url (nullable)
	if in.URL.IsNull() {
		z.Input.URL = nil
	} else if s := *in.URL.Value(); true {
		u, err := url.Parse(s)
		if err != nil {
			return nil, fieldParseErr("input.url", "valid URI", err)
		}
		z.Input.URL = &JSONURL{u}
	}

	// input.avioflags (not nullable; value required)
	if ss := *in.AVIOFlags.Value(); true {
		z.Input.AVIOFlags = make([]AVIOFlag, len(ss))
		for i, s := range ss {
			z.Input.AVIOFlags[i] = AVIOFlag(s)
		}
	}

	// input.probesize
	if v := *in.Probesize.Value(); true {
		z.Input.Probesize = v
	}

	// input.analyzeduration
	if v := *in.Analyzeduration.Value(); true {
		z.Input.Analyzeduration = v
	}

	// input.fflags (not nullable)
	if ss := *in.FFlags.Value(); true {
		z.Input.FFlags = make([]FFlag, len(ss))
		for i, s := range ss {
			z.Input.FFlags[i] = FFlag(s)
		}
	}

	// input.max_delay
	if v := *in.MaxDelay.Value(); true {
		z.Input.MaxDelay = v
	}

	// input.localaddr (nullable)
	if in.Localaddr.IsNull() {
		z.Input.Localaddr = nil
	} else if s := *in.Localaddr.Value(); true {
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return nil, fieldParseErr("input.localaddr", "IPv4 address", err)
		}
		z.Input.Localaddr = &addr
	}

	// input.timeout
	if v := *in.Timeout.Value(); true {
		z.Input.Timeout = v
	}

	// input.rtsp_transport (nullable)
	if in.RTSPTransport.IsNull() {
		z.Input.RTSPTransport = nil
	} else if s := *in.RTSPTransport.Value(); true {
		t := RTSPTransport(s)
		z.Input.RTSPTransport = &t
	}

	// output.url (nullable)
	if out.URL.IsNull() {
		z.Output.URL = nil
	} else if s := *out.URL.Value(); true {
		u, err := url.Parse(s)
		if err != nil {
			return nil, fieldParseErr("output.url", "valid URI", err)
		}
		z.Output.URL = &JSONURL{u}
	}

	// output.localaddr (nullable)
	if out.Localaddr.IsNull() {
		z.Output.Localaddr = nil
	} else if s := *out.Localaddr.Value(); true {
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return nil, fieldParseErr("output.localaddr", "IPv4 address", err)
		}
		z.Output.Localaddr = &addr
	}

	// output fields (non-nullable)
	if v := *out.PktSize.Value(); true {
		z.Output.PktSize = v
	}
	if v := *out.MapVideo.Value(); true {
		z.Output.MapVideo = v
	}
	if v := *out.MapAudio.Value(); true {
		z.Output.MapAudio = v
	}
	if v := *out.MapData.Value(); true {
		z.Output.MapData = v
	}

	// systemd/process (non-nullable)
	if v := *r.Enabled.Value(); true {
		z.Enabled = v
	}
	if v := *r.RestartSec.Value(); true {
		z.RestartSec = v
	}

	if err := z.Validate(OpCreate); err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}

	return &z, nil
}

// ===== Error helpers =====

func fieldMissingErr(path string) error {
	return fmt.Errorf("missing required field: %s", path)
}
