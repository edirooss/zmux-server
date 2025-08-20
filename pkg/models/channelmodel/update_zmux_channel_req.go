package channelmodel

import "errors"

// UpdateZmuxChannelReq is the JSON DTO for replacing a Zmux channel via PUT /api/channels/{id}.
// All fields are required (full-replacement).
// Note: The Go standard library’s json.Decoder does not differentiate
// between undefined and null values. As a result, we treat omitted
// properties (i.e., undefined fields) as null when handling nullable fields.
type UpdateZmuxChannelReq struct {
	Name *string `json:"name"` // required

	// --- Remux configuration ---
	Input  *UpdateInput  `json:"input"`  // required
	Output *UpdateOutput `json:"output"` // required
	// ----------------------------

	// Systemd settings
	Enabled    *bool `json:"enabled"`     // required
	RestartSec *uint `json:"restart_sec"` // required
}

type UpdateInput struct {
	URL             *string `json:"url"`             // required
	AVIOFlags       *string `json:"avioflags"`       // nullable
	ProbeSize       *uint   `json:"probesize"`       // required
	AnalyzeDuration *uint   `json:"analyzeduration"` // required
	FFlags          *string `json:"fflags"`          // nullable
	MaxDelay        *int    `json:"max_delay"`       // required
	LocalAddr       *string `json:"localaddr"`       // nullable
	Timeout         *uint   `json:"timeout"`         // required
	RTSPTransport   *string `json:"rtsp_transport"`  // nullable
}

type UpdateOutput struct {
	URL       *string `json:"url"`       // nullable
	LocalAddr *string `json:"localaddr"` // nullable
	PktSize   *uint   `json:"pkt_size"`  // required
	MapVideo  *bool   `json:"map_video"` // required
	MapAudio  *bool   `json:"map_audio"` // required
	MapData   *bool   `json:"map_data"`  // required
}

func (r *UpdateZmuxChannelReq) Validate() error {
	if r.Name == nil {
		return errors.New("name is required")
	}
	if r.Input == nil {
		return errors.New("input is required")
	}
	if r.Input.URL == nil {
		return errors.New("input.url is required")
	}
	if r.Input.ProbeSize == nil {
		return errors.New("input.probesize is required")
	}
	if r.Input.AnalyzeDuration == nil {
		return errors.New("input.analyzeduration is required")
	}
	if r.Input.MaxDelay == nil {
		return errors.New("input.max_delay is required")
	}
	if r.Input.Timeout == nil {
		return errors.New("input.timeout is required")
	}
	if r.Output == nil {
		return errors.New("output is required")
	}
	if r.Output.PktSize == nil {
		return errors.New("output.pkt_size is required")
	}
	if r.Output.MapVideo == nil {
		return errors.New("output.map_video is required")
	}
	if r.Output.MapAudio == nil {
		return errors.New("output.map_audio is required")
	}
	if r.Output.MapData == nil {
		return errors.New("output.map_data is required")
	}
	if r.Enabled == nil {
		return errors.New("enabled is required")
	}
	if r.RestartSec == nil {
		return errors.New("restart_sec is required")
	}
	return nil
}

// Must be used on validated requests.
func (req UpdateZmuxChannelReq) ToChannel(id int64) *ZmuxChannel {
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
