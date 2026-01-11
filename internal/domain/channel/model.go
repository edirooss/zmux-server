package channel

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

type ZmuxChannel struct {
	ID          int64               `json:"id"` //
	Interactive bool                //
	B2BClientID *int64              `json:"b2b_client_id"` // nullable
	Name        *string             `json:"name"`          // nullable
	Input       ZmuxChannelInput    `json:"input"`         //
	Outputs     []ZmuxChannelOutput `json:"outputs"`       //
	Enabled     bool                `json:"enabled"`       // (on true, input.url required)
	RestartSec  uint                `json:"restart_sec"`   //
	ReadOnly    bool                `json:"read_only"`     //
}

type ZmuxChannelInput struct {
	URL             *string `json:"url"`             // nullable (on non-null, name required)
	Username        *string `json:"username"`        // nullable (on non-null, url required)
	Password        *string `json:"password"`        // nullable (on non-null, username required)
	AVIOFlags       *string `json:"avioflags"`       // nullable
	Probesize       uint    `json:"probesize"`       //
	Analyzeduration uint    `json:"analyzeduration"` //
	FFlags          *string `json:"fflags"`          // nullable
	MaxDelay        int     `json:"max_delay"`       //
	Localaddr       *string `json:"localaddr"`       // nullable
	Timeout         uint    `json:"timeout"`         //
	RTSPTransport   *string `json:"rtsp_transport"`  // nullable
}

type ZmuxChannelOutput struct {
	Ref           string        `json:"ref"`            //
	URL           *string       `json:"url"`            // nullable
	Localaddr     *string       `json:"localaddr"`      // nullable
	PktSize       uint          `json:"pkt_size"`       //
	StreamMapping StreamMapping `json:"stream_mapping"` //
	Enabled       bool          `json:"enabled"`        // (on true, output.url required)
}

type StreamMapping []string

func (sa StreamMapping) Validate() error {
	if len(sa) == 0 {
		return fmt.Errorf("stream_mapping must include at least one media type")
	}

	for _, s := range sa {
		if s != "video" && s != "audio" && s != "data" {
			return fmt.Errorf("invalid stream_mapping value: %q (must be one of: video, audio, data)", s)
		}
	}
	return nil
}

func (sa StreamMapping) Has(target string) bool {
	for _, s := range sa {
		if s == target {
			return true
		}
	}
	return false
}

func (sa StreamMapping) HasVideo() bool { return sa.Has("video") }
func (sa StreamMapping) HasAudio() bool { return sa.Has("audio") }
func (sa StreamMapping) HasData() bool  { return sa.Has("data") }

// OutputEntry wraps a ZmuxChannelOutput with its original index
// in the Outputs slice.
type OutputEntry struct {
	Index  int
	Output ZmuxChannelOutput
}

// OutputsByRef returns the channel outputs as a map keyed by their Ref.
// Each entry includes both the output struct and its original index.
func (ch *ZmuxChannel) OutputsByRef() map[string]OutputEntry {
	outMap := make(map[string]OutputEntry, len(ch.Outputs))
	for i, o := range ch.Outputs {
		outMap[o.Ref] = OutputEntry{
			Index:  i,
			Output: o,
		}
	}
	return outMap
}

// Dependency rules
// key requires all fields in the slice
var depRules = map[string][]string{
	"input.url":      {"name"},
	"input.username": {"input.url"},
	"input.password": {"input.username"},
	"enabled":        {"input.url"},
}

func (ch *ZmuxChannel) Validate() error {
	// name: nullable, minLength 1, maxLength 100
	if ch.Name != nil {
		if len(*ch.Name) < 1 {
			return errors.New("name must be at least 1 character")
		}
		if len(*ch.Name) > 100 {
			return errors.New("name must be at most 100 characters")
		}
	}

	// input.url: uri, maxLength 2048
	if ch.Input.URL != nil {
		if len(*ch.Input.URL) > 2048 {
			return errors.New("input.url must be at most 2048 characters")
		}
		if err := validateInputURL(*ch.Input.URL); err != nil {
			return fmt.Errorf("invalid input.url: %s", err)
		}
	}

	// input.username: nullable, minLength 1, maxLength 128
	if ch.Input.Username != nil {
		if len(*ch.Input.Username) < 1 {
			return errors.New("input.username must be at least 1 character")
		}
		if len(*ch.Input.Username) > 128 {
			return errors.New("input.username must be at most 128 characters")
		}
	}

	// input.password: nullable, minLength 1, maxLength 128
	if ch.Input.Password != nil {
		if len(*ch.Input.Password) < 1 {
			return errors.New("input.password must be at least 1 character")
		}
		if len(*ch.Input.Password) > 128 {
			return errors.New("input.password must be at most 128 characters")
		}
	}

	// outputs
	outputsRefs := make(map[string]int)
	for i, output := range ch.Outputs {
		// outputs[n]: minLength 1, maxLength 100
		refLen := len(output.Ref)
		if refLen < 1 || refLen > 100 {
			return fmt.Errorf("outputs[%d].ref length must be between 1 and 128 characters", i)
		}

		// outputs[n].ref: must be unique
		if j, ok := outputsRefs[output.Ref]; ok {
			return fmt.Errorf("outputs[%d].ref must be unique (ref=%s also used at outputs[%d])", i, output.Ref, j)
		}
		outputsRefs[output.Ref] = i

		// outputs[n].url: uri
		if output.URL != nil {
			if err := validateOutputURL(*output.URL); err != nil {
				return fmt.Errorf("invalid outputs[%d].url (ref=%s): %w", i, output.Ref, err)
			}
		}

		// outputs[n].stream_mapping: must only contain valid values
		if err := output.StreamMapping.Validate(); err != nil {
			return fmt.Errorf("invalid outputs[%d].stream_mapping (ref=%s): %w", i, output.Ref, err)
		}

		if output.Enabled && output.URL == nil {
			return fmt.Errorf("outputs[%d].enabled=true missing required field outputs[%d].url", i, i)
		}
	}

	// Cross-field dependency check
	if err := ch.crossDependencyCheck(); err != nil {
		return err
	}

	return nil
}

// crossDependencyCheck ensures all required (transitive) dependencies are set for any set field in depRules.
func (ch *ZmuxChannel) crossDependencyCheck() error {
	missing := map[string]struct{}{}

	// DFS over dependencies; collect missing recursively.
	var visit func(string)
	visit = func(f string) {
		for _, dep := range depRules[f] {
			if !ch.isSet(dep) {
				missing[dep] = struct{}{}
			}
			visit(dep)
		}
	}

	// For every field that can *require* something, if it's set → enforce its deps.
	for field := range depRules {
		if ch.isSet(field) {
			visit(field)
		}
	}

	if len(missing) == 0 {
		return nil
	}
	keys := make([]string, 0, len(missing))
	for k := range missing {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return fmt.Errorf("missing (cross-dependency) required fields [%s]", strings.Join(keys, ", "))
}

// isSet returns whether a given top-level or nested field is considered "set" (i.e. non-nil or true).
//
// This function is used for validating cross-field dependencies between fields that are conditionally required.
// It implements minimal presence checks for a predefined set of fields that appear in the `depRules` map.
//
// A field is considered "set" if:
//   - It's a pointer (e.g. *string) and is non-nil
//   - It's a boolean and is true (e.g. `enabled`)
//
// NOTE: This function is tightly coupled to the `depRules` and must be kept in sync with any additions to that map.
func (ch *ZmuxChannel) isSet(field string) bool {
	switch field {
	case "name":
		return ch.Name != nil
	case "input.url":
		return ch.Input.URL != nil
	case "input.username":
		return ch.Input.Username != nil
	case "input.password":
		return ch.Input.Password != nil
	case "enabled":
		return ch.Enabled
	case "read_only":
		return ch.ReadOnly
	default:
		// Unknown fields are treated as not set; expand switch as needed.
		return false
	}
}

func (ch *ZmuxChannel) DeepClone() *ZmuxChannel {
	if ch == nil {
		return nil
	}

	clone := *ch // shallow copy first

	// Deep copy name
	if ch.Name != nil {
		nameCopy := *ch.Name
		clone.Name = &nameCopy
	}

	// Deep copy input
	clone.Input = ZmuxChannelInput{
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
	}

	// Deep copy outputs
	if len(ch.Outputs) > 0 {
		clone.Outputs = make([]ZmuxChannelOutput, len(ch.Outputs))
		for i, out := range ch.Outputs {
			clone.Outputs[i] = ZmuxChannelOutput{
				Ref:           out.Ref,
				URL:           cloneString(out.URL),
				Localaddr:     cloneString(out.Localaddr),
				PktSize:       out.PktSize,
				StreamMapping: cloneStreamMapping(out.StreamMapping),
				Enabled:       out.Enabled,
			}
		}
	}

	return &clone
}

// --- helpers ---
