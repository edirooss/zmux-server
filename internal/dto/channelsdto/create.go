package channelsdto

import (
	"errors"

	"github.com/edirooss/zmux-server/internal/domain/channel"
)

// CreateChannel is the DTO for creating a Zmux channel via
// POST /api/channels.
//   - All fields are optional.
type CreateChannel struct {
	Name       F[string]              `json:"name"`        //  optional; string | null   (default: null)
	Input      F[CreateChannelInput]  `json:"input"`       //  optional; object          (default: {})
	Output     F[CreateChannelOutput] `json:"output"`      //  optional; object          (default: {})
	Enabled    F[bool]                `json:"enabled"`     //  optional; bool            (default: false)
	RestartSec F[uint]                `json:"restart_sec"` //  optional; uint            (default: 3)
}

type CreateChannelInput struct {
	URL             F[string] `json:"url"`             //      optional; string | null   (default: null)
	AVIOFlags       F[string] `json:"avioflags"`       //      optional; string | null   (default: null)
	Probesize       F[uint]   `json:"probesize"`       //      optional; uint            (default: 5000000)
	Analyzeduration F[uint]   `json:"analyzeduration"` //      optional; uint            (default: 0)
	FFlags          F[string] `json:"fflags"`          //      optional; string | null   (default: "nobuffer")
	MaxDelay        F[int]    `json:"max_delay"`       //      optional; int             (default: -1)
	Localaddr       F[string] `json:"localaddr"`       //      optional; string | null   (default: null)
	Timeout         F[uint]   `json:"timeout"`         //      optional; uint            (default: 3000000)
	RTSPTransport   F[string] `json:"rtsp_transport"`  //      optional; string | null   (default: null)
}

type CreateChannelOutput struct {
	URL       F[string] `json:"url"`       //                  optional; string | null   (default: null)
	Localaddr F[string] `json:"localaddr"` //                  optional; string | null   (default: null)
	PktSize   F[uint]   `json:"pkt_size"`  //                  optional; uint            (default: 1316)
	MapVideo  F[bool]   `json:"map_video"` //                  optional; bool            (default: true)
	MapAudio  F[bool]   `json:"map_audio"` //                  optional; bool            (default: true)
	MapData   F[bool]   `json:"map_data"`  //                  optional; bool            (default: true)
}

// ToChannel maps CreateChannel → channel.ZmuxChannel
// Disallows explicit null assignment to non-nullable fields.
// Fills unset fields with defaults.
func (req *CreateChannel) ToChannel(id int64) (*channel.ZmuxChannel, error) {
	ch := &channel.ZmuxChannel{}
	ch.ID = id

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
