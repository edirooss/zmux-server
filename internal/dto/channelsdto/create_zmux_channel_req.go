package channelsdto

import "github.com/edirooss/zmux-server/internal/domain/channel"

// CreateZmuxChannelReq is the JSON DTO for creating a Zmux channel via POST /api/channels.
// Everything has sane defaults.
type CreateZmuxChannelReq struct {
	Name *string `json:"name"` // default: null

	// --- Remux configuration ---
	Input  *CreateInput  `json:"input"`  // default: CreateInput{}
	Output *CreateOutput `json:"output"` // default: CreateOutput{}
	// ----------------------------

	// Systemd settings
	Enabled    *bool `json:"enabled"`     // default: false
	RestartSec *uint `json:"restart_sec"` // default: 3
}

type CreateInput struct {
	URL             *string `json:"url"`             // default: null
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
		// return errors.New("name is required"); name is no longer required prop. nullable on domain
	}
	if r.Input == nil || r.Input.URL == nil {
		// return errors.New("input.url is required"); input.url is not longer required prop. nullable on domain
	}
	return nil
}

// Must be used on validated requests.
// The . derefs assume Validate() ran and ensure all required fields exists.
// If a caller forgets Validate() before ApplyDefaults(), this may panic.
func (r *CreateZmuxChannelReq) ApplyDefaults() {
	if r.Input == nil {
		r.Input = &CreateInput{}
	}
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
		r.Enabled = ptr(false)
	}
	if r.RestartSec == nil {
		r.RestartSec = ptr(uint(3))
	}
}

// Must be used on validated and filled requests.
// The * derefs assume ApplyDefaults() ran and filled all optional fields.
// If a caller forgets ApplyDefaults() before ToChannel(), this may panic.
func (req CreateZmuxChannelReq) ToChannel(id int64) *channel.ZmuxChannel {
	var ch channel.ZmuxChannel
	ch.ID = id
	ch.Name = req.Name

	ch.Input.URL = req.Input.URL
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
