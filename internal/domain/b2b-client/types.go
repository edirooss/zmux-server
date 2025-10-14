package b2bclient

// DTO (API Layer; Request schema)
type B2BClientResource struct {
	Name   string `json:"name"`
	Quotas struct {
		EnabledChannels struct {
			Quota int64 `json:"quota"`
		} `json:"enabled_channels"`
		EnabledOutputs []struct {
			Ref   string `json:"ref"`
			Quota int64  `json:"quota"`
		} `json:"enabled_outputs"`
		OnlineChannels struct {
			Quota int64 `json:"quota"`
		} `json:"online_channels"`
	} `json:"quotas"`
	ChannelIDs []int64 `json:"channel_ids"`
}

// Domain (Application Layer; Core runtime object)
type B2BClient struct {
	ID          int64
	Name        string
	BearerToken string
	Quotas      struct {
		EnabledChannels struct {
			Quota int64
			Usage int64
		}
		EnabledOutputs map[string]struct {
			Quota int64
			Usage int64
		}
		OnlineChannels struct {
			Quota int64
			Usage int64
		}
	}
	ChannelIDs []int64
}

// DB (Persistence Layer; Redis record)
type B2BClientModel struct {
	Name        string `json:"name"`
	BearerToken string `json:"bearer_token"`
	Quotas      struct {
		EnabledChannels struct {
			Quota int64 `json:"quota"`
		} `json:"enabled_channels"`
		EnabledOutputs []struct {
			Ref   string `json:"ref"`
			Quota int64  `json:"quota"`
		} `json:"enabled_outputs"`
		OnlineChannels struct {
			Quota int64 `json:"quota"`
		} `json:"online_channels"`
	} `json:"quotas"`
	ChannelIDs []int64 `json:"channel_ids"`
}

// DTO (API Layer; Response schema)
type B2BClientView struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	BearerToken string `json:"bearer_token"`
	Quotas      struct {
		EnabledChannels struct {
			Quota int64 `json:"quota"`
			Usage int64 `json:"usage"`
		} `json:"enabled_channels"`
		EnabledOutputs []struct {
			Ref   string `json:"ref"`
			Quota int64  `json:"quota"`
			Usage int64  `json:"usage"`
		} `json:"enabled_outputs"`
		OnlineChannels struct {
			Quota int64 `json:"quota"`
			Usage int64 `json:"usage"`
		} `json:"online_channels"`
	} `json:"quotas"`
	ChannelIDs []int64 `json:"channel_ids"`
}
