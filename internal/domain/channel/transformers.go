package channel

import "github.com/edirooss/zmux-server/internal/domain/channel/views"

func (ch *ZmuxChannel) AsB2BClientView() *views.ZmuxChannel {
	var outputsView map[string]views.ZmuxChannelOutput
	for _, output := range ch.Outputs {
		if outputsView == nil {
			outputsView = make(map[string]views.ZmuxChannelOutput)
		}
		ref := output.Ref
		if ref == "onprem_mr01" || ref == "onprem_mz01" || ref == "pubcloud_sky320" {
			outputsView[ref] = views.ZmuxChannelOutput{
				Enabled: output.Enabled,
			}
		}
	}

	return &views.ZmuxChannel{
		ID:   ch.ID,
		Name: ch.Name,
		Input: views.ZmuxChannelInput{
			URL:      ch.Input.URL,
			Username: ch.Input.Username,
			Password: ch.Input.Password,
		},
		Outputs: outputsView,
		Enabled: ch.Enabled,
	}
}
