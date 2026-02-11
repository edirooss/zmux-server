package dto

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/edirooss/zmux-server/internal/domain/channel"
)

// ChannelCreate is the DTO for creating a new Zmux channel via
// POST /api/channels.
//   - All fields are optional. Defaults applied.
type ChannelCreate struct {
	B2BClientID W[int64]                    `json:"b2b_client_id"` //   optional; int64  | null                       (default: null)
	Name        W[string]                   `json:"name"`          //   optional; string | null                       (default: null)
	Input       W[ChannelInputCreate]       `json:"input"`         //   optional; object                              (default: {})
	Outputs     W[[]W[ChannelOutputCreate]] `json:"outputs"`       //   optional; array[object]                       (default: [])
	Enabled     W[bool]                     `json:"enabled"`       //   optional; bool                                (default: false)
	RestartSec  W[uint]                     `json:"restart_sec"`   //   optional; uint                                (default: 3)
	ReadOnly    W[bool]                     `json:"read_only"`     //   optional; bool                                (default: false)
}

type ChannelInputCreate struct {
	URL             W[string] `json:"url"`             //       optional; string | null   (default: null)
	Username        W[string] `json:"username"`        //       optional; string | null   (default: null)
	Password        W[string] `json:"password"`        //       optional; string | null   (default: null)
	AVIOFlags       W[string] `json:"avioflags"`       //       optional; string | null   (default: null)
	Probesize       W[uint]   `json:"probesize"`       //       optional; uint            (default: 5000000)
	Analyzeduration W[uint]   `json:"analyzeduration"` //       optional; uint            (default: 0)
	FFlags          W[string] `json:"fflags"`          //       optional; string | null   (default: "nobuffer")
	MaxDelay        W[int]    `json:"max_delay"`       //       optional; int             (default: -1)
	Localaddr       W[string] `json:"localaddr"`       //       optional; string | null   (default: null)
	Timeout         W[uint]   `json:"timeout"`         //       optional; uint            (default: 3000000)
	RTSPTransport   W[string] `json:"rtsp_transport"`  //       optional; string | null   (default: null)
}

type ChannelOutputCreate struct {
	Ref           W[string]   `json:"ref"`            //                   optional; string          (default: itoa(index))
	URL           W[string]   `json:"url"`            //                   optional; string | null   (default: null)
	Localaddr     W[string]   `json:"localaddr"`      //                   optional; string | null   (default: null)
	PktSize       W[uint]     `json:"pkt_size"`       //                   optional; uint            (default: 1316)
	StreamMapping W[[]string] `json:"stream_mapping"` //                   optional; []string        (default: ["video"])
	Enabled       W[bool]     `json:"enabled"`        //                   optional; bool            (default: true)
}

// ToChannel maps CreateChannel → channel.ZmuxChannel
// Disallows explicit null assignment to non-nullable fields.
// Fills unset fields with defaults.
func (req *ChannelCreate) ToChannel() (*channel.ZmuxChannel, error) {
	ch := &channel.ZmuxChannel{}

	// b2bclnt_id
	// optional; int64 | null (default: null)
	if req.B2BClientID.Set {
		if req.B2BClientID.Null {
			ch.B2BClientID = nil
		} else {
			ch.B2BClientID = &req.B2BClientID.V
		}
	} else {
		ch.B2BClientID = nil
	}

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
		input, err := new(ChannelInputCreate).ToChannelInput()
		if err != nil {
			return nil, err
		}
		ch.Input = *input
	}

	// outputs
	// optional; array[object] (default: [])
	if req.Outputs.Set {
		if req.Outputs.Null {
			return nil, errors.New("outputs cannot be null")
		}
		outputs := req.Outputs.V
		chOutputs := make([]channel.ZmuxChannelOutput, 0)
		for i, output := range outputs {
			if output.Null {
				return nil, fmt.Errorf("outputs[%d] cannot be null", i)
			}
			chOutput, err := output.V.ToChannelOutput(i)
			if err != nil {
				return nil, fmt.Errorf("outputs[%d]: %w", i, err)
			}
			chOutputs = append(chOutputs, *chOutput)
		}
		ch.Outputs = chOutputs
	} else {
		ch.Outputs = make([]channel.ZmuxChannelOutput, 0)
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

	// readonly
	// optional; bool (default: false)
	if req.ReadOnly.Set {
		if req.ReadOnly.Null {
			return nil, errors.New("readonly cannot be null")
		}
		ch.ReadOnly = req.ReadOnly.V
	} else {
		ch.ReadOnly = false
	}

	return ch, nil
}

// ToChannelInput maps CreateChannelInput → channel.ZmuxChannelInput
// Disallows explicit null assignment to non-nullable fields.
// Fills unset fields with defaults.
func (req *ChannelInputCreate) ToChannelInput() (*channel.ZmuxChannelInput, error) {
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

	// username
	// optional; string | null (default: null)
	if req.Username.Set {
		if req.Username.Null {
			input.Username = nil
		} else {
			input.Username = &req.Username.V
		}
	} else {
		input.Username = nil
	}

	// password
	// optional; string | null (default: null)
	if req.Password.Set {
		if req.Password.Null {
			input.Password = nil
		} else {
			input.Password = &req.Password.V
		}
	} else {
		input.Password = nil
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
func (req *ChannelOutputCreate) ToChannelOutput(index int) (*channel.ZmuxChannelOutput, error) {
	output := &channel.ZmuxChannelOutput{}

	// ref
	// optional; string (default: itoa(index))
	if req.Ref.Set {
		if req.Ref.Null {
			return nil, errors.New("ref cannot be null")
		}
		output.Ref = req.Ref.V
	} else {
		output.Ref = strconv.Itoa(index)
	}

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

	// stream_mapping
	// optional; []string (default: ["video"])
	if req.StreamMapping.Set {
		if req.StreamMapping.Null {
			return nil, errors.New("stream_mapping cannot be null")
		}
		output.StreamMapping = req.StreamMapping.V
	} else {
		output.StreamMapping = channel.StreamMapping{"video"}
	}

	// enabled
	// optional; bool (default: true)
	if req.Enabled.Set {
		if req.Enabled.Null {
			return nil, errors.New("enabled cannot be null")
		}
		output.Enabled = req.Enabled.V
	} else {
		output.Enabled = true
	}

	return output, nil
}
