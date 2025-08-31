// update.go
package dto

import (
	"errors"

	"github.com/edirooss/zmux-server/internal/domain/channel"
)

// ChannelReplace is the DTO for updating a Zmux channel via
// PUT /api/channels/{id}. Full-replacement semantics (RFC 9110):
//   - All fields are required.
type ChannelReplace struct {
	Name       W[string]        `json:"name"`        //         required; string | null
	Input      W[InputReplace]  `json:"input"`       //         required; object
	Output     W[OutputReplace] `json:"output"`      //         required; object
	Enabled    W[bool]          `json:"enabled"`     //         required; bool
	RestartSec W[uint]          `json:"restart_sec"` //         required; uint
}

type InputReplace struct {
	URL             W[string] `json:"url"`             //       required; string | null
	Username        W[string] `json:"username"`        //       required; string | null
	Password        W[string] `json:"password"`        //       required; string | null
	AVIOFlags       W[string] `json:"avioflags"`       //       required; string | null
	Probesize       W[uint]   `json:"probesize"`       //       required; uint
	Analyzeduration W[uint]   `json:"analyzeduration"` //       required; uint
	FFlags          W[string] `json:"fflags"`          //       required; string | null
	MaxDelay        W[int]    `json:"max_delay"`       //       required; int
	Localaddr       W[string] `json:"localaddr"`       //       required; string | null
	Timeout         W[uint]   `json:"timeout"`         //       required; uint
	RTSPTransport   W[string] `json:"rtsp_transport"`  //       required; string | null
}

type OutputReplace struct {
	URL       W[string] `json:"url"`       //                   required; string | null
	Localaddr W[string] `json:"localaddr"` //                   required; string | null
	PktSize   W[uint]   `json:"pkt_size"`  //                   required; uint
	MapVideo  W[bool]   `json:"map_video"` //                   required; bool
	MapAudio  W[bool]   `json:"map_audio"` //                   required; bool
	MapData   W[bool]   `json:"map_data"`  //                   required; bool
}

// ToChannel maps ReplaceChannel → channel.ZmuxChannel
// Disallows explicit null assignment to non-nullable fields.
// Requires all fields to be set (PUT semantics).
func (req *ChannelReplace) ToChannel(id int64) (*channel.ZmuxChannel, error) {
	ch := &channel.ZmuxChannel{}
	ch.ID = id

	// name
	// required; string | null
	if req.Name.Set {
		if req.Name.Null {
			ch.Name = nil
		} else {
			ch.Name = &req.Name.V
		}
	} else {
		return nil, errors.New("name is required")
	}

	// input
	// required; object
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
		return nil, errors.New("input is required")
	}

	// output
	// required; object
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
		return nil, errors.New("output is required")
	}

	// enabled
	// required; bool
	if req.Enabled.Set {
		if req.Enabled.Null {
			return nil, errors.New("enabled cannot be null")
		}
		ch.Enabled = req.Enabled.V
	} else {
		return nil, errors.New("enabled is required")
	}

	// restart_sec
	// required; uint
	if req.RestartSec.Set {
		if req.RestartSec.Null {
			return nil, errors.New("restart_sec cannot be null")
		}
		ch.RestartSec = req.RestartSec.V
	} else {
		return nil, errors.New("restart_sec is required")
	}

	return ch, nil
}

// ToChannelInput maps ReplaceInput → channel.ZmuxChannelInput
// Disallows explicit null assignment to non-nullable fields.
// Requires all fields to be set (PUT semantics).
func (req *InputReplace) ToChannelInput() (*channel.ZmuxChannelInput, error) {
	input := &channel.ZmuxChannelInput{}

	// url
	// required; string | null
	if req.URL.Set {
		if req.URL.Null {
			input.URL = nil
		} else {
			input.URL = &req.URL.V
		}
	} else {
		return nil, errors.New("input.url is required")
	}

	// username
	// required; string | null
	if req.Username.Set {
		if req.Username.Null {
			input.Username = nil
		} else {
			input.Username = &req.Username.V
		}
	} else {
		return nil, errors.New("input.username is required")
	}

	// password
	// required; string | null
	if req.Password.Set {
		if req.Password.Null {
			input.Password = nil
		} else {
			input.Password = &req.Password.V
		}
	} else {
		return nil, errors.New("input.password is required")
	}

	// avioflags
	// required; string | null
	if req.AVIOFlags.Set {
		if req.AVIOFlags.Null {
			input.AVIOFlags = nil
		} else {
			input.AVIOFlags = &req.AVIOFlags.V
		}
	} else {
		return nil, errors.New("input.avioflags is required")
	}

	// probesize
	// required; uint
	if req.Probesize.Set {
		if req.Probesize.Null {
			return nil, errors.New("input.probesize cannot be null")
		}
		input.Probesize = req.Probesize.V
	} else {
		return nil, errors.New("input.probesize is required")
	}

	// analyzeduration
	// required; uint
	if req.Analyzeduration.Set {
		if req.Analyzeduration.Null {
			return nil, errors.New("input.analyzeduration cannot be null")
		}
		input.Analyzeduration = req.Analyzeduration.V
	} else {
		return nil, errors.New("input.analyzeduration is required")
	}

	// fflags
	// required; string | null
	if req.FFlags.Set {
		if req.FFlags.Null {
			input.FFlags = nil
		} else {
			input.FFlags = &req.FFlags.V
		}
	} else {
		return nil, errors.New("input.fflags is required")
	}

	// max_delay
	// required; int
	if req.MaxDelay.Set {
		if req.MaxDelay.Null {
			return nil, errors.New("input.max_delay cannot be null")
		}
		input.MaxDelay = req.MaxDelay.V
	} else {
		return nil, errors.New("input.max_delay is required")
	}

	// localaddr
	// required; string | null
	if req.Localaddr.Set {
		if req.Localaddr.Null {
			input.Localaddr = nil
		} else {
			input.Localaddr = &req.Localaddr.V
		}
	} else {
		return nil, errors.New("input.localaddr is required")
	}

	// timeout
	// required; uint
	if req.Timeout.Set {
		if req.Timeout.Null {
			return nil, errors.New("input.timeout cannot be null")
		}
		input.Timeout = req.Timeout.V
	} else {
		return nil, errors.New("input.timeout is required")
	}

	// rtsp_transport
	// required; string | null
	if req.RTSPTransport.Set {
		if req.RTSPTransport.Null {
			input.RTSPTransport = nil
		} else {
			input.RTSPTransport = &req.RTSPTransport.V
		}
	} else {
		return nil, errors.New("input.rtsp_transport is required")
	}

	return input, nil
}

// ToChannelOutput maps ReplaceOutput → channel.ZmuxChannelOutput
// Disallows explicit null assignment to non-nullable fields.
// Requires all fields to be set (PUT semantics).
func (req *OutputReplace) ToChannelOutput() (*channel.ZmuxChannelOutput, error) {
	output := &channel.ZmuxChannelOutput{}

	// url
	// required; string | null
	if req.URL.Set {
		if req.URL.Null {
			output.URL = nil
		} else {
			output.URL = &req.URL.V
		}
	} else {
		return nil, errors.New("output.url is required")
	}

	// localaddr
	// required; string | null
	if req.Localaddr.Set {
		if req.Localaddr.Null {
			output.Localaddr = nil
		} else {
			output.Localaddr = &req.Localaddr.V
		}
	} else {
		return nil, errors.New("output.localaddr is required")
	}

	// pkt_size
	// required; uint
	if req.PktSize.Set {
		if req.PktSize.Null {
			return nil, errors.New("output.pkt_size cannot be null")
		}
		output.PktSize = req.PktSize.V
	} else {
		return nil, errors.New("output.pkt_size is required")
	}

	// map_video
	// required; bool
	if req.MapVideo.Set {
		if req.MapVideo.Null {
			return nil, errors.New("output.map_video cannot be null")
		}
		output.MapVideo = req.MapVideo.V
	} else {
		return nil, errors.New("output.map_video is required")
	}

	// map_audio
	// required; bool
	if req.MapAudio.Set {
		if req.MapAudio.Null {
			return nil, errors.New("output.map_audio cannot be null")
		}
		output.MapAudio = req.MapAudio.V
	} else {
		return nil, errors.New("output.map_audio is required")
	}

	// map_data
	// required; bool
	if req.MapData.Set {
		if req.MapData.Null {
			return nil, errors.New("output.map_data cannot be null")
		}
		output.MapData = req.MapData.V
	} else {
		return nil, errors.New("output.map_data is required")
	}

	return output, nil
}
