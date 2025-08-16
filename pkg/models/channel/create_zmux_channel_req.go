package models

import "github.com/mcuadros/go-defaults"

// CreateZmuxChannelReq is the JSON DTO for creating a Zmux channel.
// Only Name and Source.InputURL are required. Everything else has sane defaults.
//
// Usage with Gin + go-defaults:
//
//	var req models.CreateZmuxChannelReq
//	req.ApplyDefaults()          // or req = models.NewCreateZmuxChannelReq()
//	if err := c.ShouldBindJSON(&req); err != nil { ... } // provided fields override defaults
//	// req.Name and req.Source.InputURL must be set by the client.
type CreateZmuxChannelReq struct {
	Name string `json:"name" binding:"required,min=1,max=100"` // must be provided by client

	// --- Remux configuration ---
	Source struct {
		InputURL        string `json:"url"       binding:"required"` // must be provided by client
		AVIOFlags       string `json:"avioflags"       default:""`
		ProbeSize       uint   `json:"probesize"       default:"5000000"`
		AnalyzeDuration uint   `json:"analyzeduration" default:"0"`
		FFlags          string `json:"fflags"          default:""`
		MaxDelay        int    `json:"max_delay"       default:"-1"`
		LocalAddr       string `json:"localaddr"       default:""`
		Timeout         uint   `json:"timeout"         default:"3000000"`
		RTSPTransport   string `json:"rtsp_transport"  default:""`
	} `json:"source"`
	Sink struct {
		OutputURL string `json:"url" default:"/dev/null"`
		LocalAddr string `json:"localaddr"  default:""`
		PktSize   uint   `json:"pkt_size"   default:"1316"`
		MapVideo  bool   `json:"map_video" default:"true"`
		MapAudio  bool   `json:"map_audio" default:"true"`
		MapData   bool   `json:"map_data"  default:"true"`
	} `json:"sink"`
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

	ch.Source.URL = req.Source.InputURL
	ch.Source.AVIOFlags = req.Source.AVIOFlags
	ch.Source.Probesize = req.Source.ProbeSize
	ch.Source.Analyzeduration = req.Source.AnalyzeDuration
	ch.Source.FFlags = req.Source.FFlags
	ch.Source.MaxDelay = req.Source.MaxDelay
	ch.Source.Localaddr = req.Source.LocalAddr
	ch.Source.Timeout = req.Source.Timeout
	ch.Source.RTSPTransport = req.Source.RTSPTransport

	ch.Sink.URL = req.Sink.OutputURL
	ch.Sink.Localaddr = req.Sink.LocalAddr
	ch.Sink.PktSize = req.Sink.PktSize
	ch.Sink.MapVideo = req.Sink.MapVideo
	ch.Sink.MapAudio = req.Sink.MapAudio
	ch.Sink.MapData = req.Sink.MapData

	ch.Enabled = req.Enabled
	ch.RestartSec = req.RestartSec
	return &ch
}
