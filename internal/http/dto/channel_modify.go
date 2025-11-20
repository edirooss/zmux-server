package dto

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/edirooss/zmux-server/internal/domain/channel"
	"github.com/edirooss/zmux-server/internal/domain/principal"
)

// ChannelModify is the DTO for updating a Zmux channel via
// PATCH /api/channels/{id}. Partial-update semantics (RFC 7386):
//   - All fields are optional.
type ChannelModify struct {
	B2BClientID W[int64]              `json:"b2b_client_id"` //   optional; int64  | null
	Name        W[string]             `json:"name"`          //   optional; string | null
	Input       W[ChannelInputModify] `json:"input"`         //   optional; object
	Outputs     W[json.RawMessage]    `json:"outputs"`       //   optional; array[object] | object[string:object]
	Enabled     W[bool]               `json:"enabled"`       //   optional; bool
	RestartSec  W[uint]               `json:"restart_sec"`   //   optional; uint
}

type ChannelInputModify struct {
	URL             W[string] `json:"url"`             //              optional; string | null
	Username        W[string] `json:"username"`        //              optional; string | null
	Password        W[string] `json:"password"`        //              optional; string | null
	AVIOFlags       W[string] `json:"avioflags"`       //              optional; string | null
	Probesize       W[uint]   `json:"probesize"`       //              optional; uint
	Analyzeduration W[uint]   `json:"analyzeduration"` //              optional; uint
	FFlags          W[string] `json:"fflags"`          //              optional; string | null
	MaxDelay        W[int]    `json:"max_delay"`       //              optional; int
	Localaddr       W[string] `json:"localaddr"`       //              optional; string | null
	Timeout         W[uint]   `json:"timeout"`         //              optional; uint
	RTSPTransport   W[string] `json:"rtsp_transport"`  //              optional; string | null
}

type ChannelOutputModify struct {
	Ref           W[string]   `json:"ref"`            //                          optional; string
	URL           W[string]   `json:"url"`            //                          optional; string | null
	Localaddr     W[string]   `json:"localaddr"`      //                          optional; string | null
	PktSize       W[uint]     `json:"pkt_size"`       //                          optional; uint
	StreamMapping W[[]string] `json:"stream_mapping"` //                          optional; []string
	Enabled       W[bool]     `json:"enabled"`        //                          optional; bool
}

// MergePatch applies ModifyChannel to channel.ZmuxChannel (in-memory)
// Disallows explicit null assignment to non-nullable fields.
// Unset fields remain unchanged.
// Enforce field-level authorization based on principal kind.
func (req *ChannelModify) MergePatch(prev *channel.ZmuxChannel, pKind principal.PrincipalKind) error {
	// b2blnt_id
	// optional; int64 | null
	// admin-only
	if req.B2BClientID.Set {
		if pKind != principal.Admin {
			return errors.New("b2blnt_id set unauthorized")
		}
		if req.B2BClientID.Null {
			prev.B2BClientID = nil
		} else {
			prev.B2BClientID = &req.B2BClientID.V
		}
	}

	// name
	// optional; string | null
	if req.Name.Set {
		if req.Name.Null {
			prev.Name = nil
		} else {
			prev.Name = &req.Name.V
		}
	}

	// input
	// optional; object
	if req.Input.Set {
		if req.Input.Null {
			return errors.New("input cannot be null")
		}
		if err := req.Input.V.MergePatch(&prev.Input, pKind); err != nil {
			return err
		}
	}

	// outputs
	// optional; array[object] | object[string:object]
	if req.Outputs.Set {
		if req.Outputs.Null {
			return errors.New("outputs cannot be null")
		}

		// array[object]
		var outputsList []W[ChannelOutputModify]
		if err := json.Unmarshal(req.Outputs.V, &outputsList); err == nil {
			// per RFC 7396 (JSON Merge Patch), arrays treated as atomic values.
			// i.e., not “merging” elements — replacing the whole array (PUT-like semantics)
			chOutputs := make([]channel.ZmuxChannelOutput, 0, len(outputsList))
			for i, output := range outputsList {
				if output.Null {
					return fmt.Errorf("outputs[%d] cannot be null", i)
				}
				chOutput, err := output.V.ToChannelOutput()
				if err != nil {
					return fmt.Errorf("outputs[%d] is invalid: %w", i, err)
				}
				chOutputs = append(chOutputs, *chOutput)
			}
			prev.Outputs = chOutputs
		} else {
			// object[string:object]
			var outputsByRef map[string]W[ChannelOutputModify]
			if err := json.Unmarshal(req.Outputs.V, &outputsByRef); err == nil {
				prevOutputsByRef := prev.OutputsByRef()
				for ref, output := range outputsByRef {
					outputEntry, ok := prevOutputsByRef[ref]
					if !ok {
						return fmt.Errorf("outputs ref %q does not exist", ref)
					}

					// Existing ref → merge
					if output.Null {
						return fmt.Errorf("outputs[%s] cannot be null", ref)
					}
					if pKind != principal.Admin &&
						(ref != "onprem_mr01" && ref != "onprem_mz01" && ref != "pubcloud_sky320") {
						return fmt.Errorf("outputs[%s] set unauthorized", ref)
					}
					if err := output.V.MergePatch(&prev.Outputs[outputEntry.Index], pKind); err != nil {
						return fmt.Errorf("outputs[%s]: %w", ref, err)
					}
				}
			} else {
				// fallback; neither array nor object
				return errors.New("outputs must be of type array or object")
			}
		}
	}

	// enabled
	// optional; bool
	if req.Enabled.Set {
		if req.Enabled.Null {
			return errors.New("enabled cannot be null")
		}
		prev.Enabled = req.Enabled.V
	}

	// restart_sec
	// optional; uint
	// admin-only
	if req.RestartSec.Set {
		if pKind != principal.Admin {
			return errors.New("restart_sec set unauthorized")
		}
		if req.RestartSec.Null {
			return errors.New("restart_sec cannot be null")
		}
		prev.RestartSec = req.RestartSec.V
	}

	return nil
}

// MergePatch applies ModifyChannelInput to channel.ZmuxChannelInput (in-memory)
// Disallows explicit null assignment to non-nullable fields.
// Unset fields remain unchanged.
// Enforce permission based on principal kind.
func (req *ChannelInputModify) MergePatch(prev *channel.ZmuxChannelInput, pKind principal.PrincipalKind) error {
	// url
	// optional; string | null
	if req.URL.Set {
		if req.URL.Null {
			prev.URL = nil
		} else {
			prev.URL = &req.URL.V
		}
	}

	// username
	// optional; string | null
	if req.Username.Set {
		if req.Username.Null {
			prev.Username = nil
		} else {
			prev.Username = &req.Username.V
		}
	}

	// password
	// optional; string | null
	if req.Password.Set {
		if req.Password.Null {
			prev.Password = nil
		} else {
			prev.Password = &req.Password.V
		}
	}

	// avioflags
	// optional; string | null
	// admin-only
	if req.AVIOFlags.Set {
		if pKind != principal.Admin {
			return errors.New("avioflags set unauthorized")
		}
		if req.AVIOFlags.Null {
			prev.AVIOFlags = nil
		} else {
			prev.AVIOFlags = &req.AVIOFlags.V
		}
	}

	// probesize
	// optional; uint
	// admin-only
	if req.Probesize.Set {
		if pKind != principal.Admin {
			return errors.New("probesize set unauthorized")
		}
		if req.Probesize.Null {
			return errors.New("probesize cannot be null")
		}
		prev.Probesize = req.Probesize.V
	}

	// analyzeduration
	// optional; uint
	// admin-only
	if req.Analyzeduration.Set {
		if pKind != principal.Admin {
			return errors.New("analyzeduration set unauthorized")
		}
		if req.Analyzeduration.Null {
			return errors.New("analyzeduration cannot be null")
		}
		prev.Analyzeduration = req.Analyzeduration.V
	}

	// fflags
	// optional; string | null
	// admin-only
	if req.FFlags.Set {
		if pKind != principal.Admin {
			return errors.New("fflags set unauthorized")
		}
		if req.FFlags.Null {
			prev.FFlags = nil
		} else {
			prev.FFlags = &req.FFlags.V
		}
	}

	// max_delay
	// optional; int
	// admin-only
	if req.MaxDelay.Set {
		if pKind != principal.Admin {
			return errors.New("max_delay set unauthorized")
		}
		if req.MaxDelay.Null {
			return errors.New("max_delay cannot be null")
		}
		prev.MaxDelay = req.MaxDelay.V
	}

	// localaddr
	// optional; string | null
	// admin-only
	if req.Localaddr.Set {
		if pKind != principal.Admin {
			return errors.New("localaddr set unauthorized")
		}
		if req.Localaddr.Null {
			prev.Localaddr = nil
		} else {
			prev.Localaddr = &req.Localaddr.V
		}
	}

	// timeout
	// optional; uint
	// admin-only
	if req.Timeout.Set {
		if pKind != principal.Admin {
			return errors.New("timeout set unauthorized")
		}
		if req.Timeout.Null {
			return errors.New("timeout cannot be null")
		}
		prev.Timeout = req.Timeout.V
	}

	// rtsp_transport
	// optional; string | null
	// admin-only
	if req.RTSPTransport.Set {
		if pKind != principal.Admin {
			return errors.New("rtsp_transport set unauthorized")
		}
		if req.RTSPTransport.Null {
			prev.RTSPTransport = nil
		} else {
			prev.RTSPTransport = &req.RTSPTransport.V
		}
	}

	return nil
}

// MergePatch applies ModifyChannelOutput to channel.ZmuxChannelOutput (in-memory)
// Disallows explicit null assignment to non-nullable fields.
// Unset fields remain unchanged.
// Enforce permission based on principal kind.
func (req *ChannelOutputModify) MergePatch(prev *channel.ZmuxChannelOutput, pKind principal.PrincipalKind) error {
	// ref
	// optional; string
	if req.Ref.Set {
		if pKind != principal.Admin {
			return errors.New("ref set unauthorized")
		}
		if req.Ref.Null {
			return errors.New("ref cannot be null")
		} else {
			prev.Ref = req.Ref.V
		}
	}

	// url
	// optional; string | null
	if req.URL.Set {
		if pKind != principal.Admin {
			return errors.New("url set unauthorized")
		}
		if req.URL.Null {
			prev.URL = nil
		} else {
			prev.URL = &req.URL.V
		}
	}

	// localaddr
	// optional; string | null
	if req.Localaddr.Set {
		if pKind != principal.Admin {
			return errors.New("localaddr set unauthorized")
		}
		if req.Localaddr.Null {
			prev.Localaddr = nil
		} else {
			prev.Localaddr = &req.Localaddr.V
		}
	}

	// pkt_size
	// optional; uint
	if req.PktSize.Set {
		if pKind != principal.Admin {
			return errors.New("pkt_size set unauthorized")
		}
		if req.PktSize.Null {
			return errors.New("pkt_size cannot be null")
		}
		prev.PktSize = req.PktSize.V
	}

	// stream_mapping
	// optional; []string
	if req.StreamMapping.Set {
		if pKind != principal.Admin {
			return errors.New("stream_mapping set unauthorized")
		}
		if req.StreamMapping.Null {
			return errors.New("stream_mapping cannot be null")
		}
		prev.StreamMapping = req.StreamMapping.V
	}

	// enabled
	// optional; bool
	if req.Enabled.Set {
		if req.Enabled.Null {
			return errors.New("enabled cannot be null")
		}
		prev.Enabled = req.Enabled.V
	}

	return nil
}

// ToChannelOutput maps ChannelOutputModify → channel.ZmuxChannelOutput
// Disallows explicit null assignment to non-nullable fields.
// JSON Merge Patch replaces arrays wholesale, it doesn’t merge them element-by-element.
// Requires all fields to be set (PUT-like semantics; require a new, complete object).
func (req *ChannelOutputModify) ToChannelOutput() (*channel.ZmuxChannelOutput, error) {
	output := &channel.ZmuxChannelOutput{}

	// ref
	// required; string
	if req.Ref.Set {
		if req.Ref.Null {
			return nil, errors.New("ref cannot be null")
		} else {
			output.Ref = req.Ref.V
		}
	} else {
		return nil, errors.New("ref is required")
	}

	// url
	// required; string | null
	if req.URL.Set {
		if req.URL.Null {
			output.URL = nil
		} else {
			output.URL = &req.URL.V
		}
	} else {
		return nil, errors.New("url is required")
	}

	// localaddr
	// required; string | null
	if req.Localaddr.Set {
		if req.Localaddr.Null {
			output.Localaddr = nil
		} else {
			output.Localaddr = &req.Localaddr.V
		}
	} else {
		return nil, errors.New("localaddr is required")
	}

	// pkt_size
	// required; uint
	if req.PktSize.Set {
		if req.PktSize.Null {
			return nil, errors.New("pkt_size cannot be null")
		}
		output.PktSize = req.PktSize.V
	} else {
		return nil, errors.New("pkt_size is required")
	}

	// stream_mapping
	// required; []string
	if req.StreamMapping.Set {
		if req.StreamMapping.Null {
			return nil, errors.New("stream_mapping cannot be null")
		}
		output.StreamMapping = req.StreamMapping.V
	} else {
		return nil, errors.New("stream_mapping is required")
	}

	// enabled
	// required; bool
	if req.Enabled.Set {
		if req.Enabled.Null {
			return nil, errors.New("enabled cannot be null")
		}
		output.Enabled = req.Enabled.V
	} else {
		return nil, errors.New("enabled is required")
	}

	return output, nil
}
