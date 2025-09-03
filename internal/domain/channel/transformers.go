package channel

import "github.com/edirooss/zmux-server/internal/domain/channel/views"

func (ch *ZmuxChannel) AsB2BClientView() *views.ZmuxChannel {
	return &views.ZmuxChannel{
		ID:   ch.ID,
		Name: ch.Name,
		Input: views.ZmuxChannelInput{
			URL:      ch.Input.URL,
			Username: ch.Input.Username,
			Password: ch.Input.Password,
		},
		Enabled: ch.Enabled,
	}
}
