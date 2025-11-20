package b2bclient

// DTO (API Layer; Request schema) — same as model minus BearerToken
type B2BClientResource struct {
	Name   string         `json:"name"`
	Quotas QuotasResource `json:"quotas"`
}

// DTO (API Layer; Response schema)
type B2BClientView struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	BearerToken string     `json:"bearer_token"`
	Quotas      QuotasView `json:"quotas"`
	ChannelIDs  []int64    `json:"channel_ids"`
}
