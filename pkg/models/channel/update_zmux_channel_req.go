// pkg/models/channel/update.go
package models

// UpdateZmuxChannelReq is a PATCH-style DTO: only provided (non-nil) fields are applied.
type UpdateZmuxChannelReq struct {
	Name *string `json:"name,omitempty" binding:"omitempty,min=1,max=100"`

	Source *struct {
		InputURL        *string `json:"url,omitempty"`
		AVIOFlags       *string `json:"avioflags,omitempty"`
		ProbeSize       *uint   `json:"probesize,omitempty"`
		AnalyzeDuration *uint   `json:"analyzeduration,omitempty"`
		FFlags          *string `json:"fflags,omitempty"`
		MaxDelay        *int    `json:"max_delay,omitempty"`
		LocalAddr       *string `json:"localaddr,omitempty"`
		Timeout         *uint   `json:"timeout,omitempty"`
		RTSPTransport   *string `json:"rtsp_transport,omitempty"`
	} `json:"input,omitempty"`

	Sink *struct {
		OutputURL *string `json:"url,omitempty"`
		LocalAddr *string `json:"localaddr,omitempty"`
		PktSize   *uint   `json:"pkt_size,omitempty"`
		MapVideo  *bool   `json:"map_video,omitempty"`
		MapAudio  *bool   `json:"map_audio,omitempty"`
		MapData   *bool   `json:"map_data,omitempty"`
	} `json:"output,omitempty"`

	Enabled    *bool `json:"enabled,omitempty"`
	RestartSec *uint `json:"restart_sec,omitempty"`
}

// ApplyTo copies non-nil fields from UpdateZmuxChannelReq into ch.
func (u *UpdateZmuxChannelReq) ApplyTo(ch *ZmuxChannel) {
	if u.Name != nil {
		ch.Name = *u.Name
	}

	if u.Source != nil {
		if u.Source.InputURL != nil {
			ch.Input.URL = *u.Source.InputURL
		}
		if u.Source.AVIOFlags != nil {
			ch.Input.AVIOFlags = *u.Source.AVIOFlags
		}
		if u.Source.ProbeSize != nil {
			ch.Input.Probesize = *u.Source.ProbeSize
		}
		if u.Source.AnalyzeDuration != nil {
			ch.Input.Analyzeduration = *u.Source.AnalyzeDuration
		}
		if u.Source.FFlags != nil {
			ch.Input.FFlags = *u.Source.FFlags
		}
		if u.Source.MaxDelay != nil {
			ch.Input.MaxDelay = *u.Source.MaxDelay
		}
		if u.Source.LocalAddr != nil {
			ch.Input.Localaddr = *u.Source.LocalAddr
		}
		if u.Source.Timeout != nil {
			ch.Input.Timeout = *u.Source.Timeout
		}
		if u.Source.RTSPTransport != nil {
			ch.Input.RTSPTransport = *u.Source.RTSPTransport
		}
	}

	if u.Sink != nil {
		if u.Sink.OutputURL != nil {
			ch.Output.URL = *u.Sink.OutputURL
		}
		if u.Sink.LocalAddr != nil {
			ch.Output.Localaddr = *u.Sink.LocalAddr
		}
		if u.Sink.PktSize != nil {
			ch.Output.PktSize = *u.Sink.PktSize
		}
		if u.Sink.MapVideo != nil {
			ch.Output.MapVideo = *u.Sink.MapVideo
		}
		if u.Sink.MapAudio != nil {
			ch.Output.MapAudio = *u.Sink.MapAudio
		}
		if u.Sink.MapData != nil {
			ch.Output.MapData = *u.Sink.MapData
		}
	}

	if u.Enabled != nil {
		ch.Enabled = *u.Enabled
	}
	if u.RestartSec != nil {
		ch.RestartSec = *u.RestartSec
	}
}
