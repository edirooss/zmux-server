// pkg/models/channel/update.go
package models

// UpdateZmuxChannelReq is a PATCH-style DTO: only provided (non-nil) fields are applied.
type UpdateZmuxChannelReq struct {
	Name *string `json:"name,omitempty" binding:"omitempty,min=1,max=100"`

	Input *struct {
		URL             *string `json:"url,omitempty"`
		AVIOFlags       *string `json:"avioflags,omitempty"`
		ProbeSize       *uint   `json:"probesize,omitempty"`
		AnalyzeDuration *uint   `json:"analyzeduration,omitempty"`
		FFlags          *string `json:"fflags,omitempty"`
		MaxDelay        *int    `json:"max_delay,omitempty"`
		LocalAddr       *string `json:"localaddr,omitempty"`
		Timeout         *uint   `json:"timeout,omitempty"`
		RTSPTransport   *string `json:"rtsp_transport,omitempty"`
	} `json:"input,omitempty"`

	Output *struct {
		URL       *string `json:"url,omitempty"`
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

	if u.Input != nil {
		if u.Input.URL != nil {
			ch.Input.URL = *u.Input.URL
		}
		if u.Input.AVIOFlags != nil {
			ch.Input.AVIOFlags = *u.Input.AVIOFlags
		}
		if u.Input.ProbeSize != nil {
			ch.Input.Probesize = *u.Input.ProbeSize
		}
		if u.Input.AnalyzeDuration != nil {
			ch.Input.Analyzeduration = *u.Input.AnalyzeDuration
		}
		if u.Input.FFlags != nil {
			ch.Input.FFlags = *u.Input.FFlags
		}
		if u.Input.MaxDelay != nil {
			ch.Input.MaxDelay = *u.Input.MaxDelay
		}
		if u.Input.LocalAddr != nil {
			ch.Input.Localaddr = *u.Input.LocalAddr
		}
		if u.Input.Timeout != nil {
			ch.Input.Timeout = *u.Input.Timeout
		}
		if u.Input.RTSPTransport != nil {
			ch.Input.RTSPTransport = *u.Input.RTSPTransport
		}
	}

	if u.Output != nil {
		if u.Output.URL != nil {
			ch.Output.URL = *u.Output.URL
		}
		if u.Output.LocalAddr != nil {
			ch.Output.Localaddr = *u.Output.LocalAddr
		}
		if u.Output.PktSize != nil {
			ch.Output.PktSize = *u.Output.PktSize
		}
		if u.Output.MapVideo != nil {
			ch.Output.MapVideo = *u.Output.MapVideo
		}
		if u.Output.MapAudio != nil {
			ch.Output.MapAudio = *u.Output.MapAudio
		}
		if u.Output.MapData != nil {
			ch.Output.MapData = *u.Output.MapData
		}
	}

	if u.Enabled != nil {
		ch.Enabled = *u.Enabled
	}
	if u.RestartSec != nil {
		ch.RestartSec = *u.RestartSec
	}
}
