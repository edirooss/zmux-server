package channelmodel

import (
	"errors"
)

// CreateZmuxChannelReq is the JSON DTO for creating a Zmux channel via POST /api/channels.
// Only Name and Input.URL are required. Everything else has sane defaults.
type CreateZmuxChannelReq struct {
	Name *string `json:"name"` // required

	// --- Remux configuration ---
	Input  *CreateInput  `json:"input"`  // required
	Output *CreateOutput `json:"output"` // default: CreateOutput{}
	// ----------------------------

	// Systemd settings
	Enabled    *bool `json:"enabled"`     // default: true
	RestartSec *uint `json:"restart_sec"` // default: 3
}

type CreateInput struct {
	URL             *string `json:"url"`             // required
	AVIOFlags       *string `json:"avioflags"`       // default: null
	ProbeSize       *uint   `json:"probesize"`       // default: 5000000
	AnalyzeDuration *uint   `json:"analyzeduration"` // default: 0
	FFlags          *string `json:"fflags"`          // default: "nobuffer" (note: overwrites explict null (i,e. defined field with null value) on create)
	MaxDelay        *int    `json:"max_delay"`       // default: -1
	LocalAddr       *string `json:"localaddr"`       // default: null
	Timeout         *uint   `json:"timeout"`         // default: 3000000
	RTSPTransport   *string `json:"rtsp_transport"`  // default: null
}

type CreateOutput struct {
	URL       *string `json:"url"`       // default: null
	LocalAddr *string `json:"localaddr"` // default: null
	PktSize   *uint   `json:"pkt_size"`  // default: 1316
	MapVideo  *bool   `json:"map_video"` // default: true
	MapAudio  *bool   `json:"map_audio"` // default: true
	MapData   *bool   `json:"map_data"`  // default: true
}

func (r *CreateZmuxChannelReq) Validate() error {
	if r.Name == nil {
		return errors.New("name is required")
	}
	if r.Input == nil || r.Input.URL == nil {
		return errors.New("input.url is required")
	}
	return nil
}

func (r *CreateZmuxChannelReq) ApplyDefaults() {
	if r.Input.ProbeSize == nil {
		r.Input.ProbeSize = ptr(uint(5000000))
	}
	if r.Input.AnalyzeDuration == nil {
		r.Input.AnalyzeDuration = ptr(uint(0))
	}
	if r.Input.FFlags == nil {
		r.Input.FFlags = ptr("nobuffer")
	}
	if r.Input.MaxDelay == nil {
		r.Input.MaxDelay = ptr(int(-1))
	}
	if r.Input.Timeout == nil {
		r.Input.Timeout = ptr(uint(3000000))
	}
	if r.Output == nil {
		r.Output = &CreateOutput{}
	}
	if r.Output.PktSize == nil {
		r.Output.PktSize = ptr(uint(1316))
	}
	if r.Output.MapVideo == nil {
		r.Output.MapVideo = ptr(true)
	}
	if r.Output.MapAudio == nil {
		r.Output.MapAudio = ptr(true)
	}
	if r.Output.MapData == nil {
		r.Output.MapData = ptr(true)
	}
	if r.Enabled == nil {
		r.Enabled = ptr(true)
	}
	if r.RestartSec == nil {
		r.RestartSec = ptr(uint(3))
	}
}

// Must be used on validated requests.
func (req CreateZmuxChannelReq) ToChannel(id int64) *ZmuxChannel {
	var ch ZmuxChannel
	ch.ID = id
	ch.Name = *req.Name

	ch.Input.URL = *req.Input.URL
	ch.Input.AVIOFlags = req.Input.AVIOFlags
	ch.Input.Probesize = *req.Input.ProbeSize
	ch.Input.Analyzeduration = *req.Input.AnalyzeDuration
	ch.Input.FFlags = req.Input.FFlags
	ch.Input.MaxDelay = *req.Input.MaxDelay
	ch.Input.Localaddr = req.Input.LocalAddr
	ch.Input.Timeout = *req.Input.Timeout
	ch.Input.RTSPTransport = req.Input.RTSPTransport

	ch.Output.URL = req.Output.URL
	ch.Output.Localaddr = req.Output.LocalAddr
	ch.Output.PktSize = *req.Output.PktSize
	ch.Output.MapVideo = *req.Output.MapVideo
	ch.Output.MapAudio = *req.Output.MapAudio
	ch.Output.MapData = *req.Output.MapData

	ch.Enabled = *req.Enabled
	ch.RestartSec = *req.RestartSec
	return &ch
}

// ptr returns a pointer to the given value.
func ptr[T any](v T) *T {
	return &v
}
