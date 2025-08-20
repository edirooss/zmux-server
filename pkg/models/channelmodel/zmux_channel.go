// models/channelmodel.go
package channelmodel

type ZmuxChannel struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`

	// --- Remux configuration ---
	Input struct {
		URL             string  `json:"url"`
		AVIOFlags       *string `json:"avioflags"`
		Probesize       uint    `json:"probesize"`
		Analyzeduration uint    `json:"analyzeduration"`
		FFlags          *string `json:"fflags"`
		MaxDelay        int     `json:"max_delay"`
		Localaddr       *string `json:"localaddr"`
		Timeout         uint    `json:"timeout"`
		RTSPTransport   *string `json:"rtsp_transport"`
	} `json:"input"`
	Output struct {
		URL       *string `json:"url"`
		Localaddr *string `json:"localaddr"`
		PktSize   uint    `json:"pkt_size"`
		MapVideo  bool    `json:"map_video"`
		MapAudio  bool    `json:"map_audio"`
		MapData   bool    `json:"map_data"`
	} `json:"output"`
	// ----------------------------

	// Systemd settings
	Enabled    bool `json:"enabled"`
	RestartSec uint `json:"restart_sec"`
}
