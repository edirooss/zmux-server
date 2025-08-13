// pkg/models/channel/update.go
package models

// UpdateZmuxChannelReq is a PATCH-style DTO: only provided (non-nil) fields are applied.
type UpdateZmuxChannelReq struct {
	Name *string `json:"name,omitempty" binding:"omitempty,min=1,max=100"`

	MapVideo *bool `json:"map_video,omitempty"`
	MapAudio *bool `json:"map_audio,omitempty"`
	MapData  *bool `json:"map_data,omitempty"`

	Demuxer *struct {
		InputURL        *string `json:"input_url,omitempty"`
		AVIOFlags       *string `json:"avioflags,omitempty"`
		ProbeSize       *uint   `json:"probesize,omitempty"`
		AnalyzeDuration *uint   `json:"analyzeduration,omitempty"`
		FFlags          *string `json:"fflags,omitempty"`
		MaxDelay        *int    `json:"max_delay,omitempty"`
		LocalAddr       *string `json:"localaddr,omitempty"`
		Timeout         *uint   `json:"timeout,omitempty"`
		RTSPTransport   *string `json:"rtsp_transport,omitempty"`
	} `json:"demuxer,omitempty"`

	Muxer *struct {
		OutputURL *string `json:"output_url,omitempty"`
		LocalAddr *string `json:"localaddr,omitempty"`
		PktSize   *uint   `json:"pkt_size,omitempty"`
	} `json:"muxer,omitempty"`

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

	if u.Demuxer != nil {
		if u.Demuxer.InputURL != nil {
			ch.Demuxer.InputURL = *u.Demuxer.InputURL
		}
		if u.Demuxer.AVIOFlags != nil {
			ch.Demuxer.AVIOFlags = *u.Demuxer.AVIOFlags
		}
		if u.Demuxer.ProbeSize != nil {
			ch.Demuxer.Probesize = *u.Demuxer.ProbeSize
		}
		if u.Demuxer.AnalyzeDuration != nil {
			ch.Demuxer.Analyzeduration = *u.Demuxer.AnalyzeDuration
		}
		if u.Demuxer.FFlags != nil {
			ch.Demuxer.FFlags = *u.Demuxer.FFlags
		}
		if u.Demuxer.MaxDelay != nil {
			ch.Demuxer.MaxDelay = *u.Demuxer.MaxDelay
		}
		if u.Demuxer.LocalAddr != nil {
			ch.Demuxer.Localaddr = *u.Demuxer.LocalAddr
		}
		if u.Demuxer.Timeout != nil {
			ch.Demuxer.Timeout = *u.Demuxer.Timeout
		}
		if u.Demuxer.RTSPTransport != nil {
			ch.Demuxer.RTSPTransport = *u.Demuxer.RTSPTransport
		}
	}

	if u.Muxer != nil {
		if u.Muxer.OutputURL != nil {
			ch.Muxer.OutputURL = *u.Muxer.OutputURL
		}
		if u.Muxer.LocalAddr != nil {
			ch.Muxer.Localaddr = *u.Muxer.LocalAddr
		}
		if u.Muxer.PktSize != nil {
			ch.Muxer.PktSize = *u.Muxer.PktSize
		}
	}

	if u.Enabled != nil {
		ch.Enabled = *u.Enabled
	}
	if u.RestartSec != nil {
		ch.RestartSec = *u.RestartSec
	}
}
