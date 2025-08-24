package channelsdto

import (
	"errors"

	"github.com/edirooss/zmux-server/internal/domain/channel"
)

// CreateChannel is the DTO for creating a Zmux channel via
// POST /api/channels.
//   - All fields are optional.
type CreateChannel struct {
	Name       F[string]       `json:"name"`        // optional; string | null (default: null)
	Input      F[CreateInput]  `json:"input"`       // optional; object (default: {})
	Output     F[CreateOutput] `json:"output"`      // optional; object (default: {})
	Enabled    F[bool]         `json:"enabled"`     // optional; bool (default: false)
	RestartSec F[uint]         `json:"restart_sec"` // optional; uint (default: 3)
}

type CreateInput struct {
	URL             F[string] `json:"url"`             // optional; string | null (default: null)
	AVIOFlags       F[string] `json:"avioflags"`       // optional; string | null (default: null)
	ProbeSize       F[uint]   `json:"probesize"`       // optional; uint (default: 5000000)
	AnalyzeDuration F[uint]   `json:"analyzeduration"` // optional; uint (default: 0)
	FFlags          F[string] `json:"fflags"`          // optional; string | null (default: "nobuffer")
	MaxDelay        F[int]    `json:"max_delay"`       // optional; int (default: -1)
	LocalAddr       F[string] `json:"localaddr"`       // optional; string | null (default: null)
	Timeout         F[uint]   `json:"timeout"`         // optional; uint (default: 3000000)
	RTSPTransport   F[string] `json:"rtsp_transport"`  // optional; string | null (default: null)
}

type CreateOutput struct {
	URL       F[string] `json:"url"`       // optional; string | null (default: null)
	LocalAddr F[string] `json:"localaddr"` // optional; string | null (default: null)
	PktSize   F[uint]   `json:"pkt_size"`  // optional; uint (default: 1316)
	MapVideo  F[bool]   `json:"map_video"` // optional; bool (default: true)
	MapAudio  F[bool]   `json:"map_audio"` // optional; bool (default: true)
	MapData   F[bool]   `json:"map_data"`  // optional; bool (default: true)
}

// Validate ensures no non-nullable field is explicitly set to null.
func (r *CreateChannel) Validate() error {
	if r.Input.Set {
		if r.Input.Null {
			return errors.New("input cannot be null")
		}
		if err := r.Input.V.Validate(); err != nil {
			return err
		}
	}
	if r.Output.Set {
		if r.Output.Null {
			return errors.New("output cannot be null")
		}
		if err := r.Output.V.Validate(); err != nil {
			return err
		}
	}

	if r.Enabled.Set && r.Enabled.Null {
		return errors.New("enabled cannot be null")
	}
	if r.RestartSec.Set && r.RestartSec.Null {
		return errors.New("restart_sec cannot be null")
	}

	return nil
}

// Validate ensures no non-nullable field is explicitly set to null.
func (c *CreateInput) Validate() error {
	if c.ProbeSize.Set && c.ProbeSize.Null {
		return errors.New("input.probesize cannot be null")
	}
	if c.AnalyzeDuration.Set && c.AnalyzeDuration.Null {
		return errors.New("input.analyzeduration cannot be null")
	}
	if c.MaxDelay.Set && c.MaxDelay.Null {
		return errors.New("input.max_delay cannot be null")
	}
	if c.Timeout.Set && c.Timeout.Null {
		return errors.New("input.timeout cannot be null")
	}
	return nil
}

func (c *CreateOutput) Validate() error {
	if c.PktSize.Set && c.PktSize.Null {
		return errors.New("output.pkt_size cannot be null")
	}
	if c.MapVideo.Set && c.MapVideo.Null {
		return errors.New("output.map_video cannot be null")
	}
	if c.MapAudio.Set && c.MapAudio.Null {
		return errors.New("output.map_audio cannot be null")
	}
	if c.MapData.Set && c.MapData.Null {
		return errors.New("output.map_data cannot be null")
	}
	return nil
}

// ApplyDefaults fills unset fields with explicit defaults.
func (r *CreateChannel) ApplyDefaults() {
	// top-level
	if !r.Name.Set {
		r.Name = NullF[string]()
	}
	if !r.Input.Set {
		r.Input = Wrap(CreateInput{})
	}
	if !r.Output.Set {
		r.Output = Wrap(CreateOutput{})
	}
	if !r.Enabled.Set {
		r.Enabled = Wrap(false)
	}
	if !r.RestartSec.Set {
		r.RestartSec = Wrap(uint(3))
	}

	// input defaults
	in := &r.Input.V
	if !in.URL.Set {
		in.URL = NullF[string]()
	}
	if !in.AVIOFlags.Set {
		in.AVIOFlags = NullF[string]()
	}
	if !in.ProbeSize.Set {
		in.ProbeSize = Wrap(uint(5000000))
	}
	if !in.AnalyzeDuration.Set {
		in.AnalyzeDuration = Wrap(uint(0))
	}
	if !in.FFlags.Set {
		in.FFlags = Wrap("nobuffer")
	}
	if !in.MaxDelay.Set {
		in.MaxDelay = Wrap(int(-1))
	}
	if !in.LocalAddr.Set {
		in.LocalAddr = NullF[string]()
	}
	if !in.Timeout.Set {
		in.Timeout = Wrap(uint(3000000))
	}
	if !in.RTSPTransport.Set {
		in.RTSPTransport = NullF[string]()
	}

	// output defaults
	out := &r.Output.V
	if !out.URL.Set {
		out.URL = NullF[string]()
	}
	if !out.LocalAddr.Set {
		out.LocalAddr = NullF[string]()
	}
	if !out.PktSize.Set {
		out.PktSize = Wrap(uint(1316))
	}
	if !out.MapVideo.Set {
		out.MapVideo = Wrap(true)
	}
	if !out.MapAudio.Set {
		out.MapAudio = Wrap(true)
	}
	if !out.MapData.Set {
		out.MapData = Wrap(true)
	}
}

// ToChannel maps CreateChannel → channel.ZmuxChannel
// Safe to call only after ApplyDefaults().
func (req CreateChannel) ToChannel(id int64) *channel.ZmuxChannel {
	var ch channel.ZmuxChannel
	ch.ID = id

	// top-level
	ch.Name = req.Name.ValueOrNil()
	ch.Enabled = req.Enabled.V
	ch.RestartSec = req.RestartSec.V

	// input
	in := &ch.Input
	in.URL = req.Input.V.URL.ValueOrNil()
	in.AVIOFlags = req.Input.V.AVIOFlags.ValueOrNil()
	in.Probesize = req.Input.V.ProbeSize.V
	in.Analyzeduration = req.Input.V.AnalyzeDuration.V
	in.FFlags = req.Input.V.FFlags.ValueOrNil()
	in.MaxDelay = req.Input.V.MaxDelay.V
	in.Localaddr = req.Input.V.LocalAddr.ValueOrNil()
	in.Timeout = req.Input.V.Timeout.V
	in.RTSPTransport = req.Input.V.RTSPTransport.ValueOrNil()

	// output
	out := &ch.Output
	out.URL = req.Output.V.URL.ValueOrNil()
	out.Localaddr = req.Output.V.LocalAddr.ValueOrNil()
	out.PktSize = req.Output.V.PktSize.V
	out.MapVideo = req.Output.V.MapVideo.V
	out.MapAudio = req.Output.V.MapAudio.V
	out.MapData = req.Output.V.MapData.V

	return &ch
}
