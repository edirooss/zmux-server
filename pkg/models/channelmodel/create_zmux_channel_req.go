package channelmodel

import "github.com/mcuadros/go-defaults"

// CreateZmuxChannelReq is the JSON DTO for creating a Zmux channel.
// Only Name and Input.InputURL are required. Everything else has sane defaults.
//
// Usage with Gin + go-defaults:
//
//	var req channelmodel.CreateZmuxChannelReq
//	req.ApplyDefaults()          // or req = channelmodel.NewCreateZmuxChannelReq()
//	if err := c.ShouldBindJSON(&req); err != nil { ... } // provided fields override defaults
//	// req.Name and req.Input.InputURL must be set by the client.
type CreateZmuxChannelReq struct {
	Name string `json:"name" binding:"required,min=1,max=100"` // must be provided by client

	// --- Remux configuration ---
	Input struct {
		URL             string `json:"url"       binding:"required"` // must be provided by client
		AVIOFlags       string `json:"avioflags"       default:""`
		ProbeSize       uint   `json:"probesize"       default:"5000000"`
		AnalyzeDuration uint   `json:"analyzeduration" default:"0"`
		FFlags          string `json:"fflags"          default:""`
		MaxDelay        int    `json:"max_delay"       default:"-1"`
		LocalAddr       string `json:"localaddr"       default:""`
		Timeout         uint   `json:"timeout"         default:"3000000"`
		RTSPTransport   string `json:"rtsp_transport"  default:""`
	} `json:"input"`
	Output struct {
		URL       string `json:"url" default:"/dev/null"`
		LocalAddr string `json:"localaddr"  default:""`
		PktSize   uint   `json:"pkt_size"   default:"1316"`
		MapVideo  bool   `json:"map_video" default:"true"`
		MapAudio  bool   `json:"map_audio" default:"true"`
		MapData   bool   `json:"map_data"  default:"true"`
	} `json:"output"`
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

	ch.Input.URL = req.Input.URL
	ch.Input.AVIOFlags = req.Input.AVIOFlags
	ch.Input.Probesize = req.Input.ProbeSize
	ch.Input.Analyzeduration = req.Input.AnalyzeDuration
	ch.Input.FFlags = req.Input.FFlags
	ch.Input.MaxDelay = req.Input.MaxDelay
	ch.Input.Localaddr = req.Input.LocalAddr
	ch.Input.Timeout = req.Input.Timeout
	ch.Input.RTSPTransport = req.Input.RTSPTransport

	ch.Output.URL = req.Output.URL
	ch.Output.Localaddr = req.Output.LocalAddr
	ch.Output.PktSize = req.Output.PktSize
	ch.Output.MapVideo = req.Output.MapVideo
	ch.Output.MapAudio = req.Output.MapAudio
	ch.Output.MapData = req.Output.MapData

	ch.Enabled = req.Enabled
	ch.RestartSec = req.RestartSec
	return &ch
}
