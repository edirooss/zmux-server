package views

type AdminZmuxChannel struct {
	ID         int64         `json:"id"`
	Name       *string       `json:"name"`
	Input      AdminInput    `json:"input"`
	Outputs    []AdminOutput `json:"outputs"`
	Enabled    bool          `json:"enabled"`
	RestartSec uint          `json:"restart_sec"`
}

type AdminInput struct {
	URL             *string `json:"url"`
	Username        *string `json:"username"`
	Password        *string `json:"password"`
	AVIOFlags       *string `json:"avioflags"`
	Probesize       uint    `json:"probesize"`
	Analyzeduration uint    `json:"analyzeduration"`
	FFlags          *string `json:"fflags"`
	MaxDelay        int     `json:"max_delay"`
	Localaddr       *string `json:"localaddr"`
	Timeout         uint    `json:"timeout"`
	RTSPTransport   *string `json:"rtsp_transport"`
}

type AdminOutput struct {
	Ref           string   `json:"ref"`
	URL           *string  `json:"url"`
	Localaddr     *string  `json:"localaddr"`
	PktSize       uint     `json:"pkt_size"`
	StreamMapping []string `json:"stream_mapping"`
	Enabled       bool     `json:"enabled"`
}
