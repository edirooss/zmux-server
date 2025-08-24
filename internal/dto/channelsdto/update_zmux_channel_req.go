package channelsdto

import (
	"errors"

	"github.com/edirooss/zmux-server/internal/domain/channel"
)

// UpdateChannel is the DTO for replacing a Zmux channel via
// PUT /api/channels/{id}. Full-replacement semantics (RFC 9110):
//   - All fields are required (must be present/defined).
type UpdateChannel struct {
	Name       F[string]       `json:"name"`        // required; string | null
	Input      F[UpdateInput]  `json:"input"`       // required; object
	Output     F[UpdateOutput] `json:"output"`      // required; object
	Enabled    F[bool]         `json:"enabled"`     // required; bool
	RestartSec F[uint]         `json:"restart_sec"` // required; uint
}

type UpdateInput struct {
	URL             F[string] `json:"url"`             // required; string | null
	AVIOFlags       F[string] `json:"avioflags"`       // required; string | null
	ProbeSize       F[uint]   `json:"probesize"`       // required; uint
	AnalyzeDuration F[uint]   `json:"analyzeduration"` // required; uint
	FFlags          F[string] `json:"fflags"`          // required; string | null
	MaxDelay        F[int]    `json:"max_delay"`       // required; int
	LocalAddr       F[string] `json:"localaddr"`       // required; string | null
	Timeout         F[uint]   `json:"timeout"`         // required; uint
	RTSPTransport   F[string] `json:"rtsp_transport"`  // required; string | null
}

type UpdateOutput struct {
	URL       F[string] `json:"url"`       // required; string | null
	LocalAddr F[string] `json:"localaddr"` // required; string | null
	PktSize   F[uint]   `json:"pkt_size"`  // required; uint
	MapVideo  F[bool]   `json:"map_video"` // required; bool
	MapAudio  F[bool]   `json:"map_audio"` // required; bool
	MapData   F[bool]   `json:"map_data"`  // required; bool
}

func (r *UpdateChannel) Validate() error {
	if !r.Name.Set {
		return errors.New("name is required")
	}

	if !r.Input.Set {
		return errors.New("input is required")
	}
	if r.Input.Null {
		return errors.New("input cannot be null")
	}
	if err := r.Input.V.Validate(); err != nil {
		return err
	}

	if !r.Output.Set {
		return errors.New("output is required")
	}
	if r.Output.Null {
		return errors.New("output cannot be null")
	}
	if err := r.Output.V.Validate(); err != nil {
		return err
	}

	if !r.Enabled.Set {
		return errors.New("enabled is required")
	}
	if r.Enabled.Null {
		return errors.New("enabled cannot be null")
	}

	if !r.RestartSec.Set {
		return errors.New("restart_sec is required")
	}
	if r.RestartSec.Null {
		return errors.New("restart_sec cannot be null")
	}
	return nil
}

func (u *UpdateInput) Validate() error {
	if !u.URL.Set {
		return errors.New("input.url is required")
	}

	if !u.AVIOFlags.Set {
		return errors.New("input.avioflags is required")
	}

	if !u.ProbeSize.Set {
		return errors.New("input.probesize is required")
	}
	if u.ProbeSize.Null {
		return errors.New("input.probesize cannot be null")
	}

	if !u.AnalyzeDuration.Set {
		return errors.New("input.analyzeduration is required")
	}
	if u.AnalyzeDuration.Null {
		return errors.New("input.analyzeduration cannot be null")
	}

	if !u.FFlags.Set {
		return errors.New("input.fflags is required")
	}

	if !u.MaxDelay.Set {
		return errors.New("input.max_delay is required")
	}
	if u.MaxDelay.Null {
		return errors.New("input.max_delay cannot be null")
	}

	if !u.LocalAddr.Set {
		return errors.New("input.localaddr is required")
	}

	if !u.Timeout.Set {
		return errors.New("input.timeout is required")
	}
	if u.Timeout.Null {
		return errors.New("input.timeout cannot be null")
	}

	if !u.RTSPTransport.Set {
		return errors.New("input.rtsp_transport is required")
	}

	return nil
}

func (u *UpdateOutput) Validate() error {
	if !u.URL.Set {
		return errors.New("output.url is required")
	}

	if !u.LocalAddr.Set {
		return errors.New("output.localaddr is required")
	}

	if !u.PktSize.Set {
		return errors.New("output.pkt_size is required")
	}
	if u.PktSize.Null {
		return errors.New("output.pkt_size cannot be null")
	}

	if !u.MapVideo.Set {
		return errors.New("output.map_video is required")
	}
	if u.MapVideo.Null {
		return errors.New("output.map_video cannot be null")
	}

	if !u.MapAudio.Set {
		return errors.New("output.map_audio is required")
	}
	if u.MapAudio.Null {
		return errors.New("output.map_audio cannot be null")
	}

	if !u.MapData.Set {
		return errors.New("output.map_data is required")
	}
	if u.MapData.Null {
		return errors.New("output.map_data cannot be null")
	}
	return nil
}

// ToChannel maps UpdateZmuxChannelReq → channel.ZmuxChannel
func (req UpdateChannel) ToChannel(id int64) *channel.ZmuxChannel {
	var ch channel.ZmuxChannel
	ch.ID = id

	// top-level fields
	if req.Name.Null {
		ch.Name = nil
	} else {
		ch.Name = &req.Name.V
	}

	ch.Enabled = req.Enabled.V
	ch.RestartSec = req.RestartSec.V

	// input mapping
	in := &ch.Input
	if req.Input.V.URL.Null {
		in.URL = nil
	} else {
		in.URL = &req.Input.V.URL.V
	}
	if req.Input.V.AVIOFlags.Null {
		in.AVIOFlags = nil
	} else {
		in.AVIOFlags = &req.Input.V.AVIOFlags.V
	}
	in.Probesize = req.Input.V.ProbeSize.V
	in.Analyzeduration = req.Input.V.AnalyzeDuration.V
	if req.Input.V.FFlags.Null {
		in.FFlags = nil
	} else {
		in.FFlags = &req.Input.V.FFlags.V
	}
	in.MaxDelay = req.Input.V.MaxDelay.V
	if req.Input.V.LocalAddr.Null {
		in.Localaddr = nil
	} else {
		in.Localaddr = &req.Input.V.LocalAddr.V
	}
	in.Timeout = req.Input.V.Timeout.V
	if req.Input.V.RTSPTransport.Null {
		in.RTSPTransport = nil
	} else {
		in.RTSPTransport = &req.Input.V.RTSPTransport.V
	}

	// output mapping
	out := &ch.Output
	if req.Output.V.URL.Null {
		out.URL = nil
	} else {
		out.URL = &req.Output.V.URL.V
	}
	if req.Output.V.LocalAddr.Null {
		out.Localaddr = nil
	} else {
		out.Localaddr = &req.Output.V.LocalAddr.V
	}
	out.PktSize = req.Output.V.PktSize.V
	out.MapVideo = req.Output.V.MapVideo.V
	out.MapAudio = req.Output.V.MapAudio.V
	out.MapData = req.Output.V.MapData.V

	return &ch
}
