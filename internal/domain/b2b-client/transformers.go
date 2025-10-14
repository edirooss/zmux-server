package b2bclient

// NewB2BClient transforms a JSON request payload into a domain-level
// B2BClient object for internal business logic.
//
// Conversion direction:
//   - API Request (DTO) → Domain
//
// This is used when receiving API input while creating a client.
func NewB2BClient(r *B2BClientResource) *B2BClient {
	if r == nil {
		return nil
	}

	enabledOutputs := make(map[string]struct {
		Quota int64
		Usage int64
	}, len(r.Quotas.EnabledOutputs))
	for _, q := range r.Quotas.EnabledOutputs {
		enabledOutputs[q.Ref] = struct {
			Quota int64
			Usage int64
		}{Quota: q.Quota}
	}

	return &B2BClient{
		Name: r.Name,
		Quotas: struct {
			EnabledChannels struct{ Quota, Usage int64 }
			EnabledOutputs  map[string]struct{ Quota, Usage int64 }
			OnlineChannels  struct{ Quota, Usage int64 }
		}{
			EnabledChannels: struct{ Quota, Usage int64 }{
				Quota: r.Quotas.EnabledChannels.Quota,
			},
			EnabledOutputs: enabledOutputs,
			OnlineChannels: struct{ Quota, Usage int64 }{
				Quota: r.Quotas.OnlineChannels.Quota,
			},
		},
		ChannelIDs: append([]int64(nil), r.ChannelIDs...),
	}
}

// Update applies fields from a B2BClientResource (API DTO; a JSON request payload) into the runtime
// domain object. This is used to apply request-driven changes during update
// flows while preserving domain invariants.
//
// Conversion direction:
//   - API Request (DTO) → Domain
//
// Overwrite semantics: all fields present in the DTO replace the receiver’s
// corresponding values.
func (c *B2BClient) Update(r *B2BClientResource) {
	if c == nil || r == nil {
		return
	}

	c.Name = r.Name

	// Quotas: EnabledChannels
	c.Quotas.EnabledChannels.Quota = r.Quotas.EnabledChannels.Quota
	c.Quotas.EnabledChannels.Usage = 0

	// Quotas: EnabledOutputs (slice → map)
	c.Quotas.EnabledOutputs = make(map[string]struct {
		Quota int64
		Usage int64
	}, len(r.Quotas.EnabledOutputs))
	for _, eo := range r.Quotas.EnabledOutputs {
		c.Quotas.EnabledOutputs[eo.Ref] = struct {
			Quota int64
			Usage int64
		}{
			Quota: eo.Quota,
			Usage: 0,
		}
	}

	// Quotas: OnlineChannels
	c.Quotas.OnlineChannels.Quota = r.Quotas.OnlineChannels.Quota
	c.Quotas.OnlineChannels.Usage = 0

	// Channels
	c.ChannelIDs = append([]int64(nil), r.ChannelIDs...)
}

// Model converts the runtime domain object into a persistence-ready record
// for database or cache storage.
//
// Conversion direction:
//   - Domain → DB (Persistence Layer)
//
// This is typically called before saving to Redis, or any persistent store.
func (c *B2BClient) Model() *B2BClientModel {
	if c == nil {
		return nil
	}

	m := &B2BClientModel{
		Name:        c.Name,
		BearerToken: c.BearerToken,
		ChannelIDs:  append([]int64(nil), c.ChannelIDs...),
	}

	// Quotas: EnabledChannels
	m.Quotas.EnabledChannels.Quota = c.Quotas.EnabledChannels.Quota

	// Quotas: EnabledOutputs (map → slice)
	m.Quotas.EnabledOutputs = make([]struct {
		Ref   string `json:"ref"`
		Quota int64  `json:"quota"`
	}, 0, len(c.Quotas.EnabledOutputs))
	for ref, eo := range c.Quotas.EnabledOutputs {
		m.Quotas.EnabledOutputs = append(m.Quotas.EnabledOutputs, struct {
			Ref   string `json:"ref"`
			Quota int64  `json:"quota"`
		}{
			Ref:   ref,
			Quota: eo.Quota,
		})
	}

	// Quotas: OnlineChannels
	m.Quotas.OnlineChannels.Quota = c.Quotas.OnlineChannels.Quota

	return m
}

// View prepares a runtime domain object for external output — typically for
// JSON serialization and HTTP responses.
//
// Conversion direction:
//   - Domain → API Response (DTO)
//
// This ensures no internal or persistence-specific fields are leaked in the API.
func (c *B2BClient) View() *B2BClientView {
	if c == nil {
		return nil
	}

	v := &B2BClientView{
		ID:          c.ID,
		Name:        c.Name,
		BearerToken: c.BearerToken,
		ChannelIDs:  append([]int64(nil), c.ChannelIDs...),
	}

	// Quotas: EnabledChannels
	v.Quotas.EnabledChannels.Quota = c.Quotas.EnabledChannels.Quota
	v.Quotas.EnabledChannels.Usage = c.Quotas.EnabledChannels.Usage

	// Quotas: EnabledOutputs (map → slice)
	v.Quotas.EnabledOutputs = make([]struct {
		Ref   string `json:"ref"`
		Quota int64  `json:"quota"`
		Usage int64  `json:"usage"`
	}, 0, len(c.Quotas.EnabledOutputs))
	for ref, eo := range c.Quotas.EnabledOutputs {
		v.Quotas.EnabledOutputs = append(v.Quotas.EnabledOutputs, struct {
			Ref   string `json:"ref"`
			Quota int64  `json:"quota"`
			Usage int64  `json:"usage"`
		}{
			Ref:   ref,
			Quota: eo.Quota,
			Usage: eo.Usage,
		})
	}

	// Quotas: OnlineChannels
	v.Quotas.OnlineChannels.Quota = c.Quotas.OnlineChannels.Quota
	v.Quotas.OnlineChannels.Usage = c.Quotas.OnlineChannels.Usage

	return v
}

// LoadB2BClient hydrates a domain B2BClient from a persistence model.
// Conversion: DB (Model) → Domain.
func LoadB2BClient(m *B2BClientModel) *B2BClient {
	if m == nil {
		return nil
	}

	c := &B2BClient{
		// ID intentionally zero; not present in the model
		Name:        m.Name,
		BearerToken: m.BearerToken,
		ChannelIDs:  append([]int64(nil), m.ChannelIDs...),
	}

	// Quotas: EnabledChannels
	c.Quotas.EnabledChannels.Quota = m.Quotas.EnabledChannels.Quota

	// Quotas: EnabledOutputs (slice → map)
	if n := len(m.Quotas.EnabledOutputs); n > 0 {
		c.Quotas.EnabledOutputs = make(map[string]struct {
			Quota int64
			Usage int64
		}, n)
		for _, eo := range m.Quotas.EnabledOutputs {
			c.Quotas.EnabledOutputs[eo.Ref] = struct {
				Quota int64
				Usage int64
			}{
				Quota: eo.Quota,
			}
		}
	} else {
		c.Quotas.EnabledOutputs = make(map[string]struct {
			Quota int64
			Usage int64
		})
	}

	// Quotas: OnlineChannels
	c.Quotas.OnlineChannels.Quota = m.Quotas.OnlineChannels.Quota

	return c
}
