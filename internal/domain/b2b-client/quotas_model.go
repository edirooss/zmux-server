package b2bclient

// ----- model layer quotas -----

type QuotasModel struct {
	EnabledChannels EnabledChannelsModel `json:"enabled_channels"`
	EnabledOutputs  []EnabledOutputModel `json:"enabled_outputs"`
	OnlineChannels  OnlineChannelsModel  `json:"online_channels"`
}

// Resource → Model (pure, copies slices)
func NewQuotasModel(r *QuotasResource) QuotasModel {
	if r == nil {
		return QuotasModel{}
	}

	outs := make([]EnabledOutputModel, len(r.EnabledOutputs))
	for i, o := range r.EnabledOutputs {
		outs[i] = EnabledOutputModel(o)
	}

	return QuotasModel{
		EnabledChannels: EnabledChannelsModel{
			Quota: r.EnabledChannels.Quota,
		},
		EnabledOutputs: outs,
		OnlineChannels: OnlineChannelsModel{
			Quota:        r.OnlineChannels.Quota,
			MaxPreflight: r.OnlineChannels.MaxPreflight,
		},
	}
}

type EnabledChannelsModel struct {
	Quota int64 `json:"quota"`
}

type EnabledOutputModel struct {
	Ref   string `json:"ref"`
	Quota int64  `json:"quota"`
}

type OnlineChannelsModel struct {
	Quota        int64 `json:"quota"`
	MaxPreflight int64 `json:"max_preflight"`
}
