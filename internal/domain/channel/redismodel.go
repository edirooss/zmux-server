package channel

// ZmuxChannelModel is a deep-copyable model representation of ZmuxChannel.
// Sub-struct types are reused, but all pointer fields and slices are cloned.
type ZmuxChannelModel struct {
	Name       *string             `json:"name"`
	Input      ZmuxChannelInput    `json:"input"`
	Outputs    []ZmuxChannelOutput `json:"outputs"`
	Enabled    bool                `json:"enabled"`
	RestartSec uint                `json:"restart_sec"`
}

// Model returns a deep-copied ZmuxChannelModel from the receiver.
// All pointer fields are reallocated, and all slices are cloned.
func (ch *ZmuxChannel) Model() ZmuxChannelModel {
	m := ZmuxChannelModel{
		Name:       cloneString(ch.Name),
		Enabled:    ch.Enabled,
		RestartSec: ch.RestartSec,
		Input: ZmuxChannelInput{
			URL:             cloneString(ch.Input.URL),
			Username:        cloneString(ch.Input.Username),
			Password:        cloneString(ch.Input.Password),
			AVIOFlags:       cloneString(ch.Input.AVIOFlags),
			Probesize:       ch.Input.Probesize,
			Analyzeduration: ch.Input.Analyzeduration,
			FFlags:          cloneString(ch.Input.FFlags),
			MaxDelay:        ch.Input.MaxDelay,
			Localaddr:       cloneString(ch.Input.Localaddr),
			Timeout:         ch.Input.Timeout,
			RTSPTransport:   cloneString(ch.Input.RTSPTransport),
		},
	}

	// Deep copy Outputs
	if len(ch.Outputs) > 0 {
		m.Outputs = make([]ZmuxChannelOutput, len(ch.Outputs))
		for i, o := range ch.Outputs {
			m.Outputs[i] = ZmuxChannelOutput{
				Ref:           o.Ref,
				URL:           cloneString(o.URL),
				Localaddr:     cloneString(o.Localaddr),
				PktSize:       o.PktSize,
				StreamMapping: cloneStreamMapping(o.StreamMapping),
				Enabled:       o.Enabled,
			}
		}
	}

	return m
}

// --- helpers ---

func cloneString(p *string) *string {
	if p == nil {
		return nil
	}
	s := *p
	return &s
}

func cloneStreamMapping(sm StreamMapping) StreamMapping {
	if sm == nil {
		return nil
	}
	out := make(StreamMapping, len(sm))
	copy(out, sm)
	return out
}
