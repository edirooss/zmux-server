package b2bclient

// DB (Persistence Layer; Redis record)
type B2BClientModel struct {
	Name        string      `json:"name"`
	BearerToken string      `json:"bearer_token"`
	Quotas      QuotasModel `json:"quotas"`
}

// API Request (Resource) + BearerToken → DB (Model)
func NewB2BClientModel(r *B2BClientResource, bearerToken string) *B2BClientModel {
	if r == nil {
		return nil
	}

	return &B2BClientModel{
		Name:        r.Name,
		BearerToken: bearerToken,
		Quotas:      NewQuotasModel(&r.Quotas),
	}
}
