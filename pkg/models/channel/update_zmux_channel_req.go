// pkg/models/channel/update.go
package models

// UpdateZmuxChannelReq is a PATCH-style DTO: only provided (non-nil) fields are applied.
type UpdateZmuxChannelReq struct {
	Name *string `json:"name,omitempty" binding:"omitempty,min=1,max=100"`

	Source *struct {
		InputURL        *string `json:"input_url,omitempty"`
		AVIOFlags       *string `json:"avioflags,omitempty"`
		ProbeSize       *uint   `json:"probesize,omitempty"`
		AnalyzeDuration *uint   `json:"analyzeduration,omitempty"`
		FFlags          *string `json:"fflags,omitempty"`
		MaxDelay        *int    `json:"max_delay,omitempty"`
		LocalAddr       *string `json:"localaddr,omitempty"`
		Timeout         *uint   `json:"timeout,omitempty"`
		RTSPTransport   *string `json:"rtsp_transport,omitempty"`
	} `json:"source,omitempty"`

	Sink *struct {
		OutputURL *string `json:"output_url,omitempty"`
		LocalAddr *string `json:"localaddr,omitempty"`
		PktSize   *uint   `json:"pkt_size,omitempty"`
	} `json:"sink,omitempty"`
	MapVideo *bool `json:"map_video,omitempty"`
	MapAudio *bool `json:"map_audio,omitempty"`
	MapData  *bool `json:"map_data,omitempty"`

	Enabled    *bool `json:"enabled,omitempty"`
	RestartSec *uint `json:"restart_sec,omitempty"`
}

// ApplyTo copies non-nil fields from UpdateZmuxChannelReq into ch.
func (u *UpdateZmuxChannelReq) ApplyTo(ch *ZmuxChannel) {
	if u.Name != nil {
		ch.Name = *u.Name
	}

	if u.MapVideo != nil {
		ch.MapVideo = *u.MapVideo
	}
	if u.MapAudio != nil {
		ch.MapAudio = *u.MapAudio
	}
	if u.MapData != nil {
		ch.MapData = *u.MapData
	}

	if u.Source != nil {
		if u.Source.InputURL != nil {
			ch.Source.InputURL = *u.Source.InputURL
		}
		if u.Source.AVIOFlags != nil {
			ch.Source.AVIOFlags = *u.Source.AVIOFlags
		}
		if u.Source.ProbeSize != nil {
			ch.Source.Probesize = *u.Source.ProbeSize
		}
		if u.Source.AnalyzeDuration != nil {
			ch.Source.Analyzeduration = *u.Source.AnalyzeDuration
		}
		if u.Source.FFlags != nil {
			ch.Source.FFlags = *u.Source.FFlags
		}
		if u.Source.MaxDelay != nil {
			ch.Source.MaxDelay = *u.Source.MaxDelay
		}
		if u.Source.LocalAddr != nil {
			ch.Source.Localaddr = *u.Source.LocalAddr
		}
		if u.Source.Timeout != nil {
			ch.Source.Timeout = *u.Source.Timeout
		}
		if u.Source.RTSPTransport != nil {
			ch.Source.RTSPTransport = *u.Source.RTSPTransport
		}
	}

	if u.Sink != nil {
		if u.Sink.OutputURL != nil {
			ch.Sink.OutputURL = *u.Sink.OutputURL
		}
		if u.Sink.LocalAddr != nil {
			ch.Sink.Localaddr = *u.Sink.LocalAddr
		}
		if u.Sink.PktSize != nil {
			ch.Sink.PktSize = *u.Sink.PktSize
		}
	}

	if u.Enabled != nil {
		ch.Enabled = *u.Enabled
	}
	if u.RestartSec != nil {
		ch.RestartSec = *u.RestartSec
	}
}
