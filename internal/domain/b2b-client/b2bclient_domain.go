package b2bclient

// Domain (Application Layer; Core runtime object)
type B2BClient struct {
	ID          int64
	Name        string
	BearerToken string
	Quotas      Quotas
}

// DB (Model) + ID → Domain
func NewB2BClient(model *B2BClientModel, id int64) *B2BClient {
	if model == nil {
		return nil
	}

	return &B2BClient{
		ID:          id,
		Name:        model.Name,
		BearerToken: model.BearerToken,
		Quotas:      NewQuotas(&model.Quotas),
	}
}

// Domain + Nested views → API Response (View)
func (c *B2BClient) View(enabledChannelsUsage int64, enabledOutputsUsage map[string]int64, onlineChannelsUsage int64, channelIDs []int64) *B2BClientView {
	if c == nil {
		return nil
	}

	return &B2BClientView{
		ID:          c.ID,
		Name:        c.Name,
		BearerToken: c.BearerToken,
		Quotas:      c.Quotas.View(enabledChannelsUsage, enabledOutputsUsage, onlineChannelsUsage),
		ChannelIDs:  append(make([]int64, 0), channelIDs...),
	}
}
