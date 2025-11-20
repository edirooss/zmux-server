package b2bclient

// ----- DTO layer quotas -----

// ----- Resources (input DTOs/API Request) -----

type QuotasResource struct {
	EnabledChannels EnabledChannelsResource `json:"enabled_channels"`
	EnabledOutputs  []EnabledOutputResource `json:"enabled_outputs"`
	OnlineChannels  OnlineChannelsResource  `json:"online_channels"`
}

type EnabledChannelsResource struct {
	Quota int64 `json:"quota"`
}

type EnabledOutputResource struct {
	Ref   string `json:"ref"`
	Quota int64  `json:"quota"`
}

type OnlineChannelsResource struct {
	Quota        int64 `json:"quota"`
	MaxPreflight int64 `json:"max_preflight"`
}

// ----- Views (output DTOs/API Response) -----

type QuotasView struct {
	EnabledChannels EnabledChannelsView `json:"enabled_channels"`
	EnabledOutputs  []EnabledOutputView `json:"enabled_outputs"`
	OnlineChannels  OnlineChannelsView  `json:"online_channels"`
}

type EnabledChannelsView struct {
	Quota int64 `json:"quota"`
	Usage int64 `json:"usage"`
}

type EnabledOutputView struct {
	Ref   string `json:"ref"`
	Quota int64  `json:"quota"`
	Usage int64  `json:"usage"`
}

type OnlineChannelsView struct {
	Quota        int64 `json:"quota"`
	MaxPreflight int64 `json:"max_preflight"`
	Usage        int64 `json:"usage"`
}
