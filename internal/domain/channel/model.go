package channel

import (
	"errors"
	"fmt"
)

type ZmuxChannel struct {
	ID         int64             `json:"id"`          //
	Name       *string           `json:"name"`        // nullable
	Input      ZmuxChannelInput  `json:"input"`       //
	Output     ZmuxChannelOutput `json:"output"`      //
	Enabled    bool              `json:"enabled"`     // (on enabled=true, requires both input.url and name to be non-null)
	RestartSec uint              `json:"restart_sec"` //
}

type ZmuxChannelInput struct {
	URL             *string `json:"url"`             // nullable (on non-null, requires name to be non-null)
	AVIOFlags       *string `json:"avioflags"`       // nullable
	Probesize       uint    `json:"probesize"`       //
	Analyzeduration uint    `json:"analyzeduration"` //
	FFlags          *string `json:"fflags"`          // nullable
	MaxDelay        int     `json:"max_delay"`       //
	Localaddr       *string `json:"localaddr"`       // nullable
	Timeout         uint    `json:"timeout"`         //
	RTSPTransport   *string `json:"rtsp_transport"`  // nullable
}

type ZmuxChannelOutput struct {
	URL       *string `json:"url"`       // nullable
	Localaddr *string `json:"localaddr"` // nullable
	PktSize   uint    `json:"pkt_size"`  //
	MapVideo  bool    `json:"map_video"` //
	MapAudio  bool    `json:"map_audio"` //
	MapData   bool    `json:"map_data"`  //
}

func (ch *ZmuxChannel) Validate() error {
	if ch.Enabled && (ch.Input.URL == nil || ch.Name == nil) {
		return errors.New("enabled=true requires non-null input.URL and name")
	}

	if ch.Input.URL != nil && ch.Name == nil {
		return errors.New("input.URL requires non-null name")
	}

	if ch.Input.URL != nil {
		if err := validateInputURL(*ch.Input.URL); err != nil {
			return fmt.Errorf("invalid input.URL: %s", err)
		}
	}

	if ch.Output.URL != nil {
		if err := validateOutputURL(*ch.Output.URL); err != nil {
			return fmt.Errorf("invalid output.URL: %s", err)
		}
	}

	return nil
}
