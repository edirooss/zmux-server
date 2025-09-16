package remuxcmd

import (
	"strconv"

	"github.com/edirooss/zmux-server/internal/domain/channel"
	"github.com/edirooss/zmux-server/pkg/avurl"
)

// BuildArgs constructs the argv slice for invoking remux.
//
// Ordering matches the CLI usage to minimize surprises:
//
//	remux [flags] --input <url> [input flags] [--output [url] [output flags]]
//
// Emission semantics:
// - All numeric/bool fields are always emitted (including 0 and false).
// - Optional strings (pointers) are emitted only when non-nil and non-empty.
// - Flags the CLI defines as StringVar are emitted via decimal strings.
func BuildArgs(c *channel.ZmuxChannel) []string {
	// Usage: remux [flags] --input <url> [input flags] [--output [url] [output flags]]

	builder := NewRemuxCommandBuilder() // pre-seeded with the binary name ("remux")

	// --- Global flags (CLI: StringVar) ---
	builder.WithStringFlag("--id", strconv.FormatInt(c.ID, 10)+"_"+strconv.FormatInt(c.Revision, 10))

	// --- Positional: --input <url> ---
	builder.WithString("--input")
	builder.WithStringP(avurl.EmbeddUserinfo(c.Input.URL, c.Input.Username, c.Input.Password))

	// --- Input flags (CLI: StringVar unless explicitly noted) ---
	builder.
		WithStringPFlag("--avioflags", c.Input.AVIOFlags).
		WithStringFlag("--probesize", strconv.FormatUint(uint64(c.Input.Probesize), 10)).
		WithStringFlag("--analyzeduration", strconv.FormatUint(uint64(c.Input.Analyzeduration), 10)).
		WithStringPFlag("--fflags", c.Input.FFlags).
		WithStringFlag("--max-delay", strconv.FormatInt(int64(c.Input.MaxDelay), 10)).
		WithStringPFlag("--localaddr", c.Input.Localaddr).
		WithStringFlag("--timeout", strconv.FormatUint(uint64(c.Input.Timeout), 10)).
		WithStringPFlag("--rtsp-transport", c.Input.RTSPTransport)

	// --- Positional/outputs section: [--output [url] [output flags]]... ---
	for _, output := range c.Outputs {
		if output.Enabled {
			builder.WithString("--output")
			builder.WithStringP(output.URL) // optional; CLI defaults to /dev/null if omitted

			// --- Output flags (CLI: StringVar for pkt-size; BoolVar for maps) ---
			builder.
				WithStringPFlag("--localaddr", output.Localaddr).
				WithStringFlag("--pkt-size", strconv.FormatUint(uint64(output.PktSize), 10)).
				WithBoolFlag("--map-video", output.StreamMapping.HasVideo()).
				WithBoolFlag("--map-audio", output.StreamMapping.HasAudio()).
				WithBoolFlag("--map-data", output.StreamMapping.HasData())
		}
	}
	return builder.BuildArgs()
}
