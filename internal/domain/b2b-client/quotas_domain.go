package b2bclient

// ---------------------
// DOMAIN QUOTA OBJECTS
// ---------------------

type Quotas struct {
	EnabledChannels EnabledChannels `json:"enabled_channels"`
	EnabledOutputs  []EnabledOutput `json:"enabled_outputs"`
	OnlineChannels  OnlineChannels  `json:"online_channels"`
}

// Model → Domain (full deep-copy, no shared memory)
func NewQuotas(model *QuotasModel) Quotas {
	if model == nil {
		return Quotas{}
	}

	outs := make([]EnabledOutput, len(model.EnabledOutputs))
	for i, o := range model.EnabledOutputs {
		outs[i] = EnabledOutput(o)
	}

	return Quotas{
		EnabledChannels: EnabledChannels{
			Quota: model.EnabledChannels.Quota,
		},
		EnabledOutputs: outs,
		OnlineChannels: OnlineChannels{
			Quota:        model.OnlineChannels.Quota,
			MaxPreflight: model.OnlineChannels.MaxPreflight,
		},
	}
}

// Domain → View (pure projection, never exposes domain memory)
func (q Quotas) View(enabledChannelsUsage int64, enabledOutputsUsage map[string]int64, onlineChannelsUsage int64) QuotasView {
	outViews := make([]EnabledOutputView, len(q.EnabledOutputs))
	for i, eo := range q.EnabledOutputs {
		outViews[i] = eo.View(enabledOutputsUsage[eo.Ref]) // always a COPY
	}

	return QuotasView{
		EnabledChannels: q.EnabledChannels.View(enabledChannelsUsage),
		EnabledOutputs:  outViews,
		OnlineChannels:  q.OnlineChannels.View(onlineChannelsUsage),
	}
}

// ------------------------
// DOMAIN SUBTYPES
// ------------------------

type EnabledChannels struct {
	Quota int64 `json:"quota"`
}

func (ec EnabledChannels) View(usage int64) EnabledChannelsView {
	return EnabledChannelsView{
		Quota: ec.Quota,
		Usage: usage,
	}
}

type EnabledOutput struct {
	Ref   string `json:"ref"`
	Quota int64  `json:"quota"`
}

func (eo EnabledOutput) View(usage int64) EnabledOutputView {
	return EnabledOutputView{Ref: eo.Ref, Quota: eo.Quota, Usage: usage}
}

type OnlineChannels struct {
	Quota        int64 `json:"quota"`
	MaxPreflight int64 `json:"max_preflight"`
}

func (oc OnlineChannels) View(usage int64) OnlineChannelsView {
	return OnlineChannelsView{
		Quota:        oc.Quota,
		MaxPreflight: oc.MaxPreflight,
		Usage:        usage,
	}
}
