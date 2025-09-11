package views

type ZmuxChannel struct {
	ID      int64                        `json:"id"`
	Name    *string                      `json:"name"`
	Input   ZmuxChannelInput             `json:"input"`
	Outputs map[string]ZmuxChannelOutput `json:"outputs"`
	Enabled bool                         `json:"enabled"`
}

type ZmuxChannelInput struct {
	URL      *string `json:"url"`
	Username *string `json:"username"`
	Password *string `json:"password"`
}

type ZmuxChannelOutput struct {
	Enabled bool `json:"enabled"`
}
