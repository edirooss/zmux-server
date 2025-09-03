package channel

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

type ZmuxChannel struct {
	ID         int64             `json:"id"`          //
	Name       *string           `json:"name"`        // nullable
	Input      ZmuxChannelInput  `json:"input"`       //
	Output     ZmuxChannelOutput `json:"output"`      //
	Enabled    bool              `json:"enabled"`     // (on true, input.url required)
	RestartSec uint              `json:"restart_sec"` //
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
	URL       *string `json:"url"`       // nullable
	Localaddr *string `json:"localaddr"` // nullable
	PktSize   uint    `json:"pkt_size"`  //
	MapVideo  bool    `json:"map_video"` //
	MapAudio  bool    `json:"map_audio"` //
	MapData   bool    `json:"map_data"`  //
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

	if ch.Output.URL != nil {
		if err := validateOutputURL(*ch.Output.URL); err != nil {
			return fmt.Errorf("invalid output.url: %s", err)
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
	default:
		// Unknown fields are treated as not set; expand switch as needed.
		return false
	}
}
