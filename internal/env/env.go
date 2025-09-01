package env

// ServiceAccountChannelIDs maps service accounts to sets of channel IDs.
type ServiceAccountChannelIDs map[string]map[int64]struct{}

// Index of service accounts with channel IDs bindings.
var ServiceAccountChannelIDsIndex = ServiceAccountChannelIDs{
	"service-account-test-1": {1: {}, 2: {}, 3: {}},
}

// Has reports whether the given service account is bound to the channel ID.
func (idx ServiceAccountChannelIDs) Has(account string, id int64) bool {
	_, ok := idx[account][id]
	return ok
}

// ChannelIDs returns all channel IDs the given service account is bound to.
func (idx ServiceAccountChannelIDs) ChannelIDs(account string) []int64 {
	ids := make([]int64, 0)
	for id := range idx[account] {
		ids = append(ids, id)
	}
	return ids
}
