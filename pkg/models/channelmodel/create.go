package channelmodel

// Create request DTOs + mapping to
// This layer is transport-facing. It:
// - captures tri-state (absent/null/value)
// - rejects nulls for non-nullable fields (schema strictness)
// - applies defaults by starting from NewZmuxChannel(0)
// - converts strings -> strong types (URL, IPv4)
// - applies business rule violation via ZmuxChannel.Validate()
import (
	"fmt"
	"net/netip"
	"net/url"

	"github.com/edirooss/zmux-server/pkg/jsonx"
)

// ---------- Create DTO ----------

// CreateZmuxChannelReq is the JSON DTO for creating a Zmux channel.
type CreateZmuxChannelReq struct {
	Name   jsonx.Field[string]       `json:"name"`   // optional (default: "New Channel"). nullable. if non-null, min=1 max=100
	Input  jsonx.Field[CreateInput]  `json:"input"`  // optional (default: object with defaults). NOT nullable
	Output jsonx.Field[CreateOutput] `json:"output"` // optional (default: object with defaults). NOT nullable

	Enabled    jsonx.Field[bool] `json:"enabled"`     // optional (default: false). if true, input.url must be non-null (domain validates)
	RestartSec jsonx.Field[uint] `json:"restart_sec"` // optional (default: 3)
}

// Remux input DTO (all fields optional; see per-field nullability).
type CreateInput struct {
	URL             jsonx.Field[string]   `json:"url"`             // optional (default: null). nullable
	AVIOFlags       jsonx.Field[[]string] `json:"avioflags"`       // optional (default: []). NOT nullable
	ProbeSize       jsonx.Field[uint]     `json:"probesize"`       // optional (default: 5_000_000). NOT nullable
	AnalyzeDuration jsonx.Field[uint]     `json:"analyzeduration"` // optional (default: 0). NOT nullable
	FFlags          jsonx.Field[[]string] `json:"fflags"`          // optional (default: ["nobuffer"]). NOT nullable
	MaxDelay        jsonx.Field[int]      `json:"max_delay"`       // optional (default: -1). NOT nullable
	LocalAddr       jsonx.Field[string]   `json:"localaddr"`       // optional (default: null). nullable (IPv4)
	Timeout         jsonx.Field[uint]     `json:"timeout"`         // optional (default: 3_000_000). NOT nullable
	RTSPTransport   jsonx.Field[string]   `json:"rtsp_transport"`  // optional (default: null). nullable ("tcp"|"udp"; domain checks)
}

// Remux output DTO (all fields optional; see per-field nullability).
type CreateOutput struct {
	URL       jsonx.Field[string] `json:"url"`       // optional (default: null). nullable
	LocalAddr jsonx.Field[string] `json:"localaddr"` // optional (default: null). nullable (IPv4)
	PktSize   jsonx.Field[uint]   `json:"pkt_size"`  // optional (default: 1316). NOT nullable
	MapVideo  jsonx.Field[bool]   `json:"map_video"` // optional (default: true). NOT nullable
	MapAudio  jsonx.Field[bool]   `json:"map_audio"` // optional (default: true). NOT nullable
	MapData   jsonx.Field[bool]   `json:"map_data"`  // optional (default: true). NOT nullable
}

// func (req *CreateZmuxChannelReq) validateRequired() {} // Could be implemented for required fields, currenly none are required.

// ---------- Mapping to domain (defaults + overlays) ----------

// ToDomain builds a ZmuxChannel with defaults applied and request values overlaid.
// It rejects explicit nulls for non-nullable fields (schema strictness) and performs
// the minimal parsing required to populate strongly-typed fields.
// Call domain validation after this:  z.Validate(OpCreate).
func (r CreateZmuxChannelReq) ToDomain() (*ZmuxChannel, error) {
	z := NewZmuxChannel(0) // defaults, ID not required for create

	// ----- name -----
	if r.Name.IsSet() {
		if r.Name.IsNull() {
			z.Name = nil
		} else if v := *r.Name.Value(); true {
			// no trim here; domain handles length 1..100
			z.Name = &v
		}
	}

	// ----- input -----
	if r.Input.IsSet() {
		if r.Input.IsNull() {
			return nil, fieldNullErr("input")
		}
		in := *r.Input.Value()
		// URL
		if in.URL.IsSet() {
			if in.URL.IsNull() {
				z.Input.URL = nil
			} else if s := *in.URL.Value(); true {
				u, err := url.Parse(s)
				if err != nil {
					return nil, fieldParseErr("input.url", "valid URI", err)
				}
				z.Input.URL = &JSONURL{u}
			}
		}
		// AVIOFlags (NOT nullable)
		if in.AVIOFlags.IsSet() {
			if in.AVIOFlags.IsNull() {
				return nil, fieldNullErr("input.avioflags")
			}
			ss := *in.AVIOFlags.Value()
			z.Input.AVIOFlags = make([]AVIOFlag, len(ss))
			for i, s := range ss {
				z.Input.AVIOFlags[i] = AVIOFlag(s)
			}
		}
		// ProbeSize (NOT nullable)
		if in.ProbeSize.IsSet() {
			if in.ProbeSize.IsNull() {
				return nil, fieldNullErr("input.probesize")
			}
			if v := *in.ProbeSize.Value(); true {
				z.Input.Probesize = v
			}
		}
		// AnalyzeDuration (NOT nullable)
		if in.AnalyzeDuration.IsSet() {
			if in.AnalyzeDuration.IsNull() {
				return nil, fieldNullErr("input.analyzeduration")
			}
			if v := *in.AnalyzeDuration.Value(); true {
				z.Input.Analyzeduration = v
			}
		}
		// FFlags (NOT nullable)
		if in.FFlags.IsSet() {
			if in.FFlags.IsNull() {
				return nil, fieldNullErr("input.fflags")
			}
			ss := *in.FFlags.Value()
			z.Input.FFlags = make([]FFlag, len(ss))
			for i, s := range ss {
				z.Input.FFlags[i] = FFlag(s)
			}
		}
		// MaxDelay (NOT nullable)
		if in.MaxDelay.IsSet() {
			if in.MaxDelay.IsNull() {
				return nil, fieldNullErr("input.max_delay")
			}
			if v := *in.MaxDelay.Value(); true {
				z.Input.MaxDelay = v
			}
		}
		// LocalAddr (nullable)
		if in.LocalAddr.IsSet() {
			if in.LocalAddr.IsNull() {
				z.Input.Localaddr = nil
			} else if s := *in.LocalAddr.Value(); true {
				addr, err := netip.ParseAddr(s)
				if err != nil {
					return nil, fieldParseErr("input.localaddr", "IPv4 address", err)
				}
				// Domain will enforce IPv4 via z.Validate
				z.Input.Localaddr = &addr
			}
		}
		// Timeout (NOT nullable)
		if in.Timeout.IsSet() {
			if in.Timeout.IsNull() {
				return nil, fieldNullErr("input.timeout")
			}
			if v := *in.Timeout.Value(); true {
				z.Input.Timeout = v
			}
		}
		// RTSPTransport (nullable)
		if in.RTSPTransport.IsSet() {
			if in.RTSPTransport.IsNull() {
				z.Input.RTSPTransport = nil
			} else if s := *in.RTSPTransport.Value(); true {
				t := RTSPTransport(s) // domain will check allowed values
				z.Input.RTSPTransport = &t
			}
		}
	}

	// ----- output -----
	if r.Output.IsSet() {
		if r.Output.IsNull() {
			return nil, fieldNullErr("output")
		}
		out := *r.Output.Value()
		// URL
		if out.URL.IsSet() {
			if out.URL.IsNull() {
				z.Output.URL = nil
			} else if s := *out.URL.Value(); true {
				u, err := url.Parse(s)
				if err != nil {
					return nil, fieldParseErr("output.url", "valid URI", err)
				}
				z.Output.URL = &JSONURL{u}
			}
		}
		// LocalAddr (nullable)
		if out.LocalAddr.IsSet() {
			if out.LocalAddr.IsNull() {
				z.Output.Localaddr = nil
			} else if s := *out.LocalAddr.Value(); true {
				addr, err := netip.ParseAddr(s)
				if err != nil {
					return nil, fieldParseErr("output.localaddr", "IPv4 address", err)
				}
				z.Output.Localaddr = &addr
			}
		}
		// PktSize (NOT nullable)
		if out.PktSize.IsSet() {
			if out.PktSize.IsNull() {
				return nil, fieldNullErr("output.pkt_size")
			}
			if v := *out.PktSize.Value(); true {
				z.Output.PktSize = v
			}
		}
		// Map* (NOT nullable)
		if out.MapVideo.IsSet() {
			if out.MapVideo.IsNull() {
				return nil, fieldNullErr("output.map_video")
			}
			if v := *out.MapVideo.Value(); true {
				z.Output.MapVideo = v
			}
		}
		if out.MapAudio.IsSet() {
			if out.MapAudio.IsNull() {
				return nil, fieldNullErr("output.map_audio")
			}
			if v := *out.MapAudio.Value(); true {
				z.Output.MapAudio = v
			}
		}
		if out.MapData.IsSet() {
			if out.MapData.IsNull() {
				return nil, fieldNullErr("output.map_data")
			}
			if v := *out.MapData.Value(); true {
				z.Output.MapData = v
			}
		}
	}

	// ----- systemd bits -----
	if r.Enabled.IsSet() {
		if r.Enabled.IsNull() {
			return nil, fieldNullErr("enabled")
		}
		if v := *r.Enabled.Value(); true {
			z.Enabled = v
		}
	}
	if r.RestartSec.IsSet() {
		if r.RestartSec.IsNull() {
			return nil, fieldNullErr("restart_sec")
		}
		if v := *r.RestartSec.Value(); true {
			z.RestartSec = v
		}
	}

	if err := z.Validate(OpCreate); err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}

	return &z, nil
}

// ---------- Error helpers ----------

func fieldNullErr(path string) error {
	return fmt.Errorf("%s cannot be null", path)
}

func fieldParseErr(path, want string, cause error) error {
	return fmt.Errorf("%s must be %s: %v", path, want, cause)
}
