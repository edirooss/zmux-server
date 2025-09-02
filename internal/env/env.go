package env

// B2BClientChannelIDs maps b2b clients to sets of channel IDs.
type B2BClientChannelIDs map[string]map[int64]struct{}

// Index of b2b clients with channel IDs bindings.
var B2BClientChannelIDsIndex = B2BClientChannelIDs{
	"b2b-client-test-1": {1: {}, 2: {}, 3: {}},
}

// Has reports whether the given b2b client is bound to the channel ID.
func (idx B2BClientChannelIDs) Has(clientID string, channelID int64) bool {
	_, ok := idx[clientID][channelID]
	return ok
}

// ChannelIDs returns all channel IDs the given b2b client is bound to.
func (idx B2BClientChannelIDs) ChannelIDs(clientID string) []int64 {
	channelIDs := make([]int64, 0)
	for channelID := range idx[clientID] {
		channelIDs = append(channelIDs, channelID)
	}
	return channelIDs
}
