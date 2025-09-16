package views

type B2BClientZmuxChannel struct {
	ID      int64                      `json:"id"`
	Name    *string                    `json:"name"`
	Input   B2BClientInput             `json:"input"`
	Outputs map[string]B2BClientOutput `json:"outputs"`
	Enabled bool                       `json:"enabled"`
}

type B2BClientInput struct {
	URL      *string `json:"url"`
	Username *string `json:"username"`
	Password *string `json:"password"`
}

type B2BClientOutput struct {
	Enabled bool `json:"enabled"`
}
