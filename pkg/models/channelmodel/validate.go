// File: validate.go
// Validate holds allowed values.
package channelmodel

import (
	"fmt"
	"net/url"
	"strings"
)

// Operation context (ID requirement is an op precondition, not a data invariant).
type Op int

const (
	OpCreate  Op = iota
	OpReplace    // PUT
	OpPatch      // PATCH
)

// Bounds & constants (spec-derived)
const (
	minNameLen = 1
	maxNameLen = 100

	minProbeSize       = 32
	maxProbeSize       = 50_000_000
	minAnalyzeDuration = 0
	maxAnalyzeDuration = 60_000_000
	minMaxDelay        = -1
	maxMaxDelay        = 10_000_000
	minTimeout         = 0
	maxTimeout         = 120_000_000

	minPktSize = 188
	maxPktSize = 1316

	minRestartSec = 0
	maxRestartSec = 120
)

// ValidationError aggregates field errors for precise responses.
type ValidationError struct {
	Problems map[string]string
}

func (v *ValidationError) Error() string {
	if len(v.Problems) == 0 {
		return "no validation errors"
	}

	var keys []string
	for k := range v.Problems {
		keys = append(keys, k)
	}

	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s: %s; ", k, v.Problems[k])
	}
	out := strings.TrimSuffix(b.String(), "; ")
	return fmt.Sprintf("validation failed (%d problem(s)); %s", len(v.Problems), out)
}

func (v *ValidationError) add(field, msg string) {
	if v.Problems == nil {
		v.Problems = make(map[string]string)
	}
	// Last-one-wins; if you want multi-errors per field, switch to []string.
	v.Problems[field] = msg
}
func (v *ValidationError) empty() bool { return len(v.Problems) == 0 }

// Validate runs all invariants. Pure function: no I/O, no DB.
func (z *ZmuxChannel) Validate(op Op) error {
	ve := &ValidationError{}

	// Operation preconditions
	if op != OpCreate && z.ID == 0 {
		ve.add("id", "must be set for update")
	}

	// ----- Name -----
	// Required if input.url is set OR enabled == true, otherwise nullable.
	nameRequired := z.Enabled || z.Input.URL != nil
	if z.Name == nil {
		if nameRequired {
			ve.add("name", "is required when input.url is set or enabled is true")
		}
	} else {
		nl := len(*z.Name)
		if nl < minNameLen || nl > maxNameLen {
			ve.add("name", fmt.Sprintf("length must be between %d and %d", minNameLen, maxNameLen))
		}
	}

	// ----- Input.URL -----
	if z.Input.URL != nil {
		if msg := validateURI(z.Input.URL.URL); msg != "" {
			ve.add("input.url", msg)
		}
	}

	// If enabled == true ⇒ input.url is required (and already validated above)
	if z.Enabled && z.Input.URL == nil {
		ve.add("input.url", "is required when enabled is true")
	}

	// ----- Input enums/lists -----
	for i, f := range z.Input.AVIOFlags {
		switch f {
		case AVIOFlagDirect:
		default:
			ve.add(fmt.Sprintf("input.avioflags[%d]", i), `invalid value (allowed: "direct")`)
		}
	}
	for i, f := range z.Input.FFlags {
		switch f {
		case FFlagNoBuffer:
		default:
			ve.add(fmt.Sprintf("input.fflags[%d]", i), `invalid value (allowed: "nobuffer")`)
		}
	}

	// ----- Input numeric bounds -----
	if z.Input.Probesize < minProbeSize || z.Input.Probesize > maxProbeSize {
		ve.add("input.probesize", fmt.Sprintf("must be between %d and %d", minProbeSize, maxProbeSize))
	}
	if z.Input.Analyzeduration < minAnalyzeDuration || z.Input.Analyzeduration > maxAnalyzeDuration {
		ve.add("input.analyzeduration", fmt.Sprintf("must be between %d and %d", minAnalyzeDuration, maxAnalyzeDuration))
	}
	if z.Input.MaxDelay < minMaxDelay || z.Input.MaxDelay > maxMaxDelay {
		ve.add("input.max_delay", fmt.Sprintf("must be between %d and %d", minMaxDelay, maxMaxDelay))
	}
	if z.Input.Timeout < minTimeout || z.Input.Timeout > maxTimeout {
		ve.add("input.timeout", fmt.Sprintf("must be between %d and %d", minTimeout, maxTimeout))
	}

	// ----- Input addresses / transport -----
	if z.Input.Localaddr != nil && !z.Input.Localaddr.Is4() {
		ve.add("input.localaddr", "must be a valid IPv4 address")
	}
	if z.Input.RTSPTransport != nil {
		switch *z.Input.RTSPTransport {
		case RTSPTransportTCP, RTSPTransportUDP, RTSPTransportUDPMulticast:
		default:
			ve.add("input.rtsp_transport", `invalid value (allowed: "tcp"|"udp"|"udp_multicast")`)
		}
	}

	// ----- Output.URL -----
	if z.Output.URL != nil {
		if msg := validateURI(z.Output.URL.URL); msg != "" {
			ve.add("output.url", msg)
		}
	}

	// ----- Output address -----
	if z.Output.Localaddr != nil && !z.Output.Localaddr.Is4() {
		ve.add("output.localaddr", "must be a valid IPv4 address")
	}

	// ----- Output numeric -----
	if z.Output.PktSize < minPktSize || z.Output.PktSize > maxPktSize {
		ve.add("output.pkt_size", fmt.Sprintf("must be between %d and %d", minPktSize, maxPktSize))
	}

	// ----- Systemd / process -----
	if z.RestartSec < minRestartSec || z.RestartSec > maxRestartSec {
		ve.add("restart_sec", fmt.Sprintf("must be between %d and %d", minRestartSec, maxRestartSec))
	}

	if ve.empty() {
		return nil
	}
	return ve
}

// validateURI: "valid URI" per spec = must have scheme; must have host or opaque.
// This covers common cases (rtsp://host/..., http://..., file:opaque, etc.).
func validateURI(u *url.URL) string {
	if u.Scheme == "" {
		return "must have a non-empty URI scheme"
	}
	if u.Host == "" && u.Opaque == "" && u.Path == "" {
		return "must include host, opaque, or path component"
	}
	return ""
}
