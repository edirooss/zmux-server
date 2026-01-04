package channel

import "github.com/edirooss/zmux-server/internal/domain/channel/views"

func (ch *ZmuxChannel) B2BClientView() *views.B2BClientZmuxChannel {
	var outputsView map[string]views.B2BClientOutput
	for _, output := range ch.Outputs {
		if outputsView == nil {
			outputsView = make(map[string]views.B2BClientOutput)
		}
		ref := output.Ref
		if ref == "onprem_mr01" || ref == "onprem_mz01" || ref == "pubcloud_sky320" {
			outputsView[ref] = views.B2BClientOutput{
				Enabled: output.Enabled,
			}
		}
	}

	return &views.B2BClientZmuxChannel{
		ID:   ch.ID,
		Name: ch.Name,
		Input: views.B2BClientInput{
			URL:      ch.Input.URL,
			Username: ch.Input.Username,
			Password: ch.Input.Password,
		},
		Outputs: outputsView,
		Enabled: ch.Enabled,
	}
}

// AdminView returns the admin-facing view of the channel.
func (ch *ZmuxChannel) AdminView() *views.AdminZmuxChannel {
	// Build outputs slice
	outputsView := make([]views.AdminOutput, len(ch.Outputs))
	for i, output := range ch.Outputs {
		outputsView[i] = views.AdminOutput{
			Ref:           output.Ref,
			URL:           output.URL,
			Localaddr:     output.Localaddr,
			PktSize:       output.PktSize,
			StreamMapping: output.StreamMapping,
			Enabled:       output.Enabled,
		}
	}

	return &views.AdminZmuxChannel{
		ID:          ch.ID,
		B2BClientID: ch.B2BClientID,
		Name:        ch.Name,
		Input: views.AdminInput{
			URL:             ch.Input.URL,
			Username:        ch.Input.Username,
			Password:        ch.Input.Password,
			AVIOFlags:       ch.Input.AVIOFlags,
			Probesize:       ch.Input.Probesize,
			Analyzeduration: ch.Input.Analyzeduration,
			FFlags:          ch.Input.FFlags,
			MaxDelay:        ch.Input.MaxDelay,
			Localaddr:       ch.Input.Localaddr,
			Timeout:         ch.Input.Timeout,
			RTSPTransport:   ch.Input.RTSPTransport,
		},
		Outputs:    outputsView,
		Enabled:    ch.Enabled,
		RestartSec: ch.RestartSec,
		ReadOnly:   ch.ReadOnly,
	}
}
