package dto

import (
	"errors"

	"github.com/edirooss/zmux-server/internal/domain/channel"
)

// ModifyChannel is the DTO for updating a Zmux channel via
// PATCH /api/channels/{id}. Partial-update semantics (RFC 7386):
//   - All fields are optional.
type ModifyChannel struct {
	Name       W[string]              `json:"name"`        //   optional; string | null
	Input      W[ModifyChannelInput]  `json:"input"`       //   optional; object
	Output     W[ModifyChannelOutput] `json:"output"`      //   optional; object
	Enabled    W[bool]                `json:"enabled"`     //   optional; bool
	RestartSec W[uint]                `json:"restart_sec"` //   optional; uint
}

type ModifyChannelInput struct {
	URL             W[string] `json:"url"`             //              optional; string | null
	AVIOFlags       W[string] `json:"avioflags"`       //              optional; string | null
	Probesize       W[uint]   `json:"probesize"`       //              optional; uint
	Analyzeduration W[uint]   `json:"analyzeduration"` //              optional; uint
	FFlags          W[string] `json:"fflags"`          //              optional; string | null
	MaxDelay        W[int]    `json:"max_delay"`       //              optional; int
	Localaddr       W[string] `json:"localaddr"`       //              optional; string | null
	Timeout         W[uint]   `json:"timeout"`         //              optional; uint
	RTSPTransport   W[string] `json:"rtsp_transport"`  //              optional; string | null
}

type ModifyChannelOutput struct {
	URL       W[string] `json:"url"`       //                          optional; string | null
	Localaddr W[string] `json:"localaddr"` //                          optional; string | null
	PktSize   W[uint]   `json:"pkt_size"`  //                          optional; uint
	MapVideo  W[bool]   `json:"map_video"` //                          optional; bool
	MapAudio  W[bool]   `json:"map_audio"` //                          optional; bool
	MapData   W[bool]   `json:"map_data"`  //                          optional; bool
}

// MergePatch applies ModifyChannel to channel.ZmuxChannel (in-memory)
// Disallows explicit null assignment to non-nullable fields.
// Unset fields remain unchanged.
func (req *ModifyChannel) MergePatch(prev *channel.ZmuxChannel) error {
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
		if err := req.Input.V.MergePatch(&prev.Input); err != nil {
			return err
		}
	}

	// output
	// optional; object
	if req.Output.Set {
		if req.Output.Null {
			return errors.New("output cannot be null")
		}
		if err := req.Output.V.MergePatch(&prev.Output); err != nil {
			return err
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
	if req.RestartSec.Set {
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
func (req *ModifyChannelInput) MergePatch(prev *channel.ZmuxChannelInput) error {
	// url
	// optional; string | null
	if req.URL.Set {
		if req.URL.Null {
			prev.URL = nil
		} else {
			prev.URL = &req.URL.V
		}
	}

	// avioflags
	// optional; string | null
	if req.AVIOFlags.Set {
		if req.AVIOFlags.Null {
			prev.AVIOFlags = nil
		} else {
			prev.AVIOFlags = &req.AVIOFlags.V
		}
	}

	// probesize
	// optional; uint
	if req.Probesize.Set {
		if req.Probesize.Null {
			return errors.New("probesize cannot be null")
		}
		prev.Probesize = req.Probesize.V
	}

	// analyzeduration
	// optional; uint
	if req.Analyzeduration.Set {
		if req.Analyzeduration.Null {
			return errors.New("analyzeduration cannot be null")
		}
		prev.Analyzeduration = req.Analyzeduration.V
	}

	// fflags
	// optional; string | null
	if req.FFlags.Set {
		if req.FFlags.Null {
			prev.FFlags = nil
		} else {
			prev.FFlags = &req.FFlags.V
		}
	}

	// max_delay
	// optional; int
	if req.MaxDelay.Set {
		if req.MaxDelay.Null {
			return errors.New("max_delay cannot be null")
		}
		prev.MaxDelay = req.MaxDelay.V
	}

	// localaddr
	// optional; string | null
	if req.Localaddr.Set {
		if req.Localaddr.Null {
			prev.Localaddr = nil
		} else {
			prev.Localaddr = &req.Localaddr.V
		}
	}

	// timeout
	// optional; uint
	if req.Timeout.Set {
		if req.Timeout.Null {
			return errors.New("timeout cannot be null")
		}
		prev.Timeout = req.Timeout.V
	}

	// rtsp_transport
	// optional; string | null
	if req.RTSPTransport.Set {
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
func (req *ModifyChannelOutput) MergePatch(prev *channel.ZmuxChannelOutput) error {
	// url
	// optional; string | null
	if req.URL.Set {
		if req.URL.Null {
			prev.URL = nil
		} else {
			prev.URL = &req.URL.V
		}
	}

	// localaddr
	// optional; string | null
	if req.Localaddr.Set {
		if req.Localaddr.Null {
			prev.Localaddr = nil
		} else {
			prev.Localaddr = &req.Localaddr.V
		}
	}

	// pkt_size
	// optional; uint
	if req.PktSize.Set {
		if req.PktSize.Null {
			return errors.New("pkt_size cannot be null")
		}
		prev.PktSize = req.PktSize.V
	}

	// map_video
	// optional; bool
	if req.MapVideo.Set {
		if req.MapVideo.Null {
			return errors.New("map_video cannot be null")
		}
		prev.MapVideo = req.MapVideo.V
	}

	// map_audio
	// optional; bool
	if req.MapAudio.Set {
		if req.MapAudio.Null {
			return errors.New("map_audio cannot be null")
		}
		prev.MapAudio = req.MapAudio.V
	}

	// map_data
	// optional; bool
	if req.MapData.Set {
		if req.MapData.Null {
			return errors.New("map_data cannot be null")
		}
		prev.MapData = req.MapData.V
	}

	return nil
}
