// models/models.go
package models

import "github.com/mcuadros/go-defaults"

// CreateZmuxChannelReq is the JSON DTO for creating a Zmux channel.
// Only Name and Demuxer.InputURL are required. Everything else has sane defaults.
//
// Usage with Gin + go-defaults:
//
//	var req models.CreateZmuxChannelReq
//	req.ApplyDefaults()          // or req = models.NewCreateZmuxChannelReq()
//	if err := c.ShouldBindJSON(&req); err != nil { ... } // provided fields override defaults
//	// req.Name and req.Demuxer.InputURL must be set by the client.
type CreateZmuxChannelReq struct {
	Name string `json:"name" binding:"required,min=1,max=100"` // must be provided by client

	// --- Remux configuration ---
	MapVideo bool `json:"map_video" default:"true"`
	MapAudio bool `json:"map_audio" default:"true"`
	MapData  bool `json:"map_data"  default:"true"`
	Demuxer  struct {
		InputURL        string `json:"input_url"       binding:"required"` // must be provided by client
		AVIOFlags       string `json:"avioflags"       default:""`
		ProbeSize       uint   `json:"probesize"       default:"5000000"`
		AnalyzeDuration uint   `json:"analyzeduration" default:"0"`
		FFlags          string `json:"fflags"          default:""`
		MaxDelay        int    `json:"max_delay"       default:"-1"`
		LocalAddr       string `json:"localaddr"       default:""`
		Timeout         uint   `json:"timeout"         default:"3000000"`
		RTSPTransport   string `json:"rtsp_transport"  default:""`
	} `json:"demuxer"`
	Muxer struct {
		OutputURL string `json:"output_url" default:"/dev/null"`
		LocalAddr string `json:"localaddr"  default:""`
		PktSize   uint   `json:"pkt_size"   default:"1316"`
	} `json:"muxer"`
	// ----------------------------

	// Systemd settings
	Enabled    bool `json:"enabled"    default:"true"`
	RestartSec uint `json:"restart_sec" default:"3"`
}

// NewCreateZmuxChannelReq returns a struct pre-filled with defaults.
func NewCreateZmuxChannelReq() CreateZmuxChannelReq {
	var r CreateZmuxChannelReq
	defaults.SetDefaults(&r)
	return r
}

func (req CreateZmuxChannelReq) ToChannel(id int64) *ZmuxChannel {
	var ch ZmuxChannel
	ch.ID = id
	ch.Name = req.Name

	ch.MapVideo = req.MapVideo
	ch.MapAudio = req.MapAudio
	ch.MapData = req.MapData

	ch.Demuxer.InputURL = req.Demuxer.InputURL
	ch.Demuxer.AVIOFlags = req.Demuxer.AVIOFlags
	ch.Demuxer.Probesize = req.Demuxer.ProbeSize
	ch.Demuxer.Analyzeduration = req.Demuxer.AnalyzeDuration
	ch.Demuxer.FFlags = req.Demuxer.FFlags
	ch.Demuxer.MaxDelay = req.Demuxer.MaxDelay
	ch.Demuxer.Localaddr = req.Demuxer.LocalAddr
	ch.Demuxer.Timeout = req.Demuxer.Timeout
	ch.Demuxer.RTSPTransport = req.Demuxer.RTSPTransport

	ch.Muxer.OutputURL = req.Muxer.OutputURL
	ch.Muxer.Localaddr = req.Muxer.LocalAddr
	ch.Muxer.PktSize = req.Muxer.PktSize

	ch.Enabled = req.Enabled
	ch.RestartSec = req.RestartSec
	return &ch
}
