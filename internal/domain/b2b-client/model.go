package b2bclient

type B2BClient struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	BearerToken string `json:"bearer_token"`

	EnabledChannelQuota int64 `json:"enabled_channel_quota"`
	EnabledOutputQuotas []struct {
		Ref string `json:"ref"`
		Val int64  `json:"val"`
	} `json:"enabled_output_quotas"`
	OnlineChannelQuota int64 `json:"online_channel_quota"`

	ChannelIDs []int64 `json:"channel_ids"`
}
