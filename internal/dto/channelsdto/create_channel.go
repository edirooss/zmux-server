package channelsdto

import (
	"errors"

	"github.com/edirooss/zmux-server/internal/domain/channel"
)

// CreateChannel is the DTO for creating a new Zmux channel via
// POST /api/channels.
//   - All fields are optional. Defaults applied.
type CreateChannel struct {
	Name       W[string]              `json:"name"`        //   optional; string | null   (default: null)
	Input      W[CreateChannelInput]  `json:"input"`       //   optional; object          (default: {})
	Output     W[CreateChannelOutput] `json:"output"`      //   optional; object          (default: {})
	Enabled    W[bool]                `json:"enabled"`     //   optional; bool            (default: false)
	RestartSec W[uint]                `json:"restart_sec"` //   optional; uint            (default: 3)
}

type CreateChannelInput struct {
	URL             W[string] `json:"url"`             //       optional; string | null   (default: null)
	AVIOFlags       W[string] `json:"avioflags"`       //       optional; string | null   (default: null)
	Probesize       W[uint]   `json:"probesize"`       //       optional; uint            (default: 5000000)
	Analyzeduration W[uint]   `json:"analyzeduration"` //       optional; uint            (default: 0)
	FFlags          W[string] `json:"fflags"`          //       optional; string | null   (default: "nobuffer")
	MaxDelay        W[int]    `json:"max_delay"`       //       optional; int             (default: -1)
	Localaddr       W[string] `json:"localaddr"`       //       optional; string | null   (default: null)
	Timeout         W[uint]   `json:"timeout"`         //       optional; uint            (default: 3000000)
	RTSPTransport   W[string] `json:"rtsp_transport"`  //       optional; string | null   (default: null)
}

type CreateChannelOutput struct {
	URL       W[string] `json:"url"`       //                   optional; string | null   (default: null)
	Localaddr W[string] `json:"localaddr"` //                   optional; string | null   (default: null)
	PktSize   W[uint]   `json:"pkt_size"`  //                   optional; uint            (default: 1316)
	MapVideo  W[bool]   `json:"map_video"` //                   optional; bool            (default: true)
	MapAudio  W[bool]   `json:"map_audio"` //                   optional; bool            (default: true)
	MapData   W[bool]   `json:"map_data"`  //                   optional; bool            (default: true)
}

// ToChannel maps CreateChannel → channel.ZmuxChannel
// Disallows explicit null assignment to non-nullable fields.
// Fills unset fields with defaults.
func (req *CreateChannel) ToChannel() (*channel.ZmuxChannel, error) {
	ch := &channel.ZmuxChannel{}

	// name
	// optional; string | null (default: null)
	if req.Name.Set {
		if req.Name.Null {
			ch.Name = nil
		} else {
			ch.Name = &req.Name.V
		}
	} else {
		ch.Name = nil
	}

	// input
	// optional; object (default: {})
	if req.Input.Set {
		if req.Input.Null {
			return nil, errors.New("input cannot be null")
		}
		input, err := req.Input.V.ToChannelInput()
		if err != nil {
			return nil, err
		}
		ch.Input = *input
	} else {
		input, err := new(CreateChannelInput).ToChannelInput()
		if err != nil {
			return nil, err
		}
		ch.Input = *input
	}

	// output
	// optional; object (default: {})
	if req.Output.Set {
		if req.Output.Null {
			return nil, errors.New("output cannot be null")
		}
		output, err := req.Output.V.ToChannelOutput()
		if err != nil {
			return nil, err
		}
		ch.Output = *output
	} else {
		output, err := new(CreateChannelOutput).ToChannelOutput()
		if err != nil {
			return nil, err
		}
		ch.Output = *output
	}

	// enabled
	// optional; bool (default: false)
	if req.Enabled.Set {
		if req.Enabled.Null {
			return nil, errors.New("enabled cannot be null")
		}
		ch.Enabled = req.Enabled.V
	} else {
		ch.Enabled = false
	}

	// restart_sec
	// optional; uint (default: 3)
	if req.RestartSec.Set {
		if req.RestartSec.Null {
			return nil, errors.New("restart_sec cannot be null")
		}
		ch.RestartSec = req.RestartSec.V
	} else {
		ch.RestartSec = 3
	}

	return ch, nil
}

// ToChannelInput maps CreateChannelInput → channel.ZmuxChannelInput
// Disallows explicit null assignment to non-nullable fields.
// Fills unset fields with defaults.
func (req *CreateChannelInput) ToChannelInput() (*channel.ZmuxChannelInput, error) {
	input := &channel.ZmuxChannelInput{}

	// url
	// optional; string | null (default: null)
	if req.URL.Set {
		if req.URL.Null {
			input.URL = nil
		} else {
			input.URL = &req.URL.V
		}
	} else {
		input.URL = nil
	}

	// avioflags
	// optional; string | null (default: null)
	if req.AVIOFlags.Set {
		if req.AVIOFlags.Null {
			input.AVIOFlags = nil
		} else {
			input.AVIOFlags = &req.AVIOFlags.V
		}
	} else {
		input.AVIOFlags = nil
	}

	// probesize
	// optional; uint (default: 5000000)
	if req.Probesize.Set {
		if req.Probesize.Null {
			return nil, errors.New("probesize cannot be null")
		}
		input.Probesize = req.Probesize.V
	} else {
		input.Probesize = 5000000
	}

	// analyzeduration
	// optional; uint (default: 0)
	if req.Analyzeduration.Set {
		if req.Analyzeduration.Null {
			return nil, errors.New("analyzeduration cannot be null")
		}
		input.Analyzeduration = req.Analyzeduration.V
	} else {
		input.Analyzeduration = 0
	}

	// fflags
	// optional; string | null (default: "nobuffer")
	if req.FFlags.Set {
		if req.FFlags.Null {
			input.FFlags = nil
		} else {
			input.FFlags = &req.FFlags.V
		}
	} else {
		def := "nobuffer"
		input.FFlags = &def
	}

	// max_delay
	// optional; int (default: -1)
	if req.MaxDelay.Set {
		if req.MaxDelay.Null {
			return nil, errors.New("max_delay cannot be null")
		}
		input.MaxDelay = req.MaxDelay.V
	} else {
		input.MaxDelay = -1
	}

	// localaddr
	// optional; string | null (default: null)
	if req.Localaddr.Set {
		if req.Localaddr.Null {
			input.Localaddr = nil
		} else {
			input.Localaddr = &req.Localaddr.V
		}
	} else {
		input.Localaddr = nil
	}

	// timeout
	// optional; uint (default: 3000000)
	if req.Timeout.Set {
		if req.Timeout.Null {
			return nil, errors.New("timeout cannot be null")
		}
		input.Timeout = req.Timeout.V
	} else {
		input.Timeout = 3000000
	}

	// rtsp_transport
	// optional; string | null (default: null)
	if req.RTSPTransport.Set {
		if req.RTSPTransport.Null {
			input.RTSPTransport = nil
		} else {
			input.RTSPTransport = &req.RTSPTransport.V
		}
	} else {
		input.RTSPTransport = nil
	}

	return input, nil
}

// ToChannelOutput maps CreateChannelOutput → channel.ZmuxChannelOutput
// Disallows explicit null assignment to non-nullable fields.
// Fills unset fields with defaults.
func (req *CreateChannelOutput) ToChannelOutput() (*channel.ZmuxChannelOutput, error) {
	output := &channel.ZmuxChannelOutput{}

	// url
	// optional; string | null (default: null)
	if req.URL.Set {
		if req.URL.Null {
			output.URL = nil
		} else {
			output.URL = &req.URL.V
		}
	} else {
		output.URL = nil
	}

	// localaddr
	// optional; string | null (default: null)
	if req.Localaddr.Set {
		if req.Localaddr.Null {
			output.Localaddr = nil
		} else {
			output.Localaddr = &req.Localaddr.V
		}
	} else {
		output.Localaddr = nil
	}

	// pkt_size
	// optional; uint (default: 1316)
	if req.PktSize.Set {
		if req.PktSize.Null {
			return nil, errors.New("pkt_size cannot be null")
		}
		output.PktSize = req.PktSize.V
	} else {
		output.PktSize = 1316
	}

	// map_video
	// optional; bool (default: true)
	if req.MapVideo.Set {
		if req.MapVideo.Null {
			return nil, errors.New("map_video cannot be null")
		}
		output.MapVideo = req.MapVideo.V
	} else {
		output.MapVideo = true
	}

	// map_audio
	// optional; bool (default: true)
	if req.MapAudio.Set {
		if req.MapAudio.Null {
			return nil, errors.New("map_audio cannot be null")
		}
		output.MapAudio = req.MapAudio.V
	} else {
		output.MapAudio = true
	}

	// map_data
	// optional; bool (default: true)
	if req.MapData.Set {
		if req.MapData.Null {
			return nil, errors.New("map_data cannot be null")
		}
		output.MapData = req.MapData.V
	} else {
		output.MapData = true
	}

	return output, nil
}
