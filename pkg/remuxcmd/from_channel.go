package remuxcmd

import (
	"strconv"

	"github.com/edirooss/zmux-server/internal/domain/channel"
	"github.com/edirooss/zmux-server/pkg/avurl"
)

// FromChannel materializes a Builder from a domain-level ZmuxChannel.
//
// It encodes CLI policy for `remux`:
//
//	remux [flags] --input <url> [input flags] [--output [url] [output flags]]...
//
// Ordering is stable to minimize operational surprises when diffing commands.
//
// Emission semantics:
// - All numeric/bool fields are always emitted (including 0 and false).
// - Optional strings (pointers) are emitted only when non-nil and non-empty.
// - Flags the CLI defines as StringVar are emitted via decimal strings.
//
// NOTE: This function does *not* validate domain fields; it encodes them.
// Validation belongs in the domain layer.
func FromChannel(c *channel.ZmuxChannel) *Builder {
	b := NewBuilder()

	// --- Global flags (CLI: StringVar) ---
	b.WithStringFlag("--id", strconv.FormatInt(c.ID, 10))

	// --- Positional: --input <url> ---
	b.WithString("--input")
	b.WithStringP(avurl.EmbeddUserinfo(c.Input.URL, c.Input.Username, c.Input.Password))

	// --- Input flags (CLI: StringVar unless noted) ---
	b.
		WithStringPFlag("--avioflags", c.Input.AVIOFlags).
		WithStringFlag("--probesize", strconv.FormatUint(uint64(c.Input.Probesize), 10)).
		WithStringFlag("--analyzeduration", strconv.FormatUint(uint64(c.Input.Analyzeduration), 10)).
		WithStringPFlag("--fflags", c.Input.FFlags).
		WithStringFlag("--max-delay", strconv.FormatInt(int64(c.Input.MaxDelay), 10)). // int → decimal string
		WithStringPFlag("--localaddr", c.Input.Localaddr).
		WithStringFlag("--timeout", strconv.FormatUint(uint64(c.Input.Timeout), 10)).
		WithStringPFlag("--rtsp-transport", c.Input.RTSPTransport)

	// --- Outputs section: [--output [url] [output flags]]... ---
	for _, out := range c.Outputs {
		if !out.Enabled {
			continue
		}
		b.WithString("--output")
		// Optional URL: CLI defaults to /dev/null if omitted.
		b.WithStringP(out.URL)

		// Output flags (pkt-size: StringVar; maps: BoolVar)
		b.
			WithStringPFlag("--localaddr", out.Localaddr).
			WithStringFlag("--pkt-size", strconv.FormatUint(uint64(out.PktSize), 10)).
			WithBoolFlag("--map-video", out.StreamMapping.HasVideo()).
			WithBoolFlag("--map-audio", out.StreamMapping.HasAudio()).
			WithBoolFlag("--map-data", out.StreamMapping.HasData())
	}

	return b
}
