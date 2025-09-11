package remuxcmd

import (
	"strconv"

	"github.com/edirooss/zmux-server/internal/domain/channel"
	"github.com/edirooss/zmux-server/pkg/avurl"
)

// BuildRemuxExec maps channel.ZmuxChannel → safe exec string for systemd and POSIX shells.
//
// Ordering matches the CLI usage to minimize surprises:
//
//	remux [flags] --input <url> [input flags] [--output [url] [output flags]]
//
// Emission semantics:
// - All numeric/bool fields are always emitted (including 0 and false).
// - Optional strings (pointers) are emitted only when non-nil and non-empty.
// - Flags the CLI defines as StringVar are emitted via decimal strings.
func BuildRemuxExec(ch *channel.ZmuxChannel) string {
	// Usage: remux [flags] --input <url> [input flags] [--output [url] [output flags]]
	builder := NewRemuxCommandBuilder()

	// --- Global flags (CLI: StringVar) ---
	builder.WithStringFlag("--id", strconv.FormatInt(ch.ID, 10))

	// --- Positional: --input <url> ---
	builder.WithString("--input")
	builder.WithStringP(avurl.EmbeddUserinfo(ch.Input.URL, ch.Input.Username, ch.Input.Password))

	// --- Input flags (CLI: StringVar unless explicitly noted) ---
	builder.
		WithStringPFlag("--avioflags", ch.Input.AVIOFlags).
		WithStringFlag("--probesize", strconv.FormatUint(uint64(ch.Input.Probesize), 10)).
		WithStringFlag("--analyzeduration", strconv.FormatUint(uint64(ch.Input.Analyzeduration), 10)).
		WithStringPFlag("--fflags", ch.Input.FFlags).
		WithStringFlag("--max-delay", strconv.FormatInt(int64(ch.Input.MaxDelay), 10)).
		WithStringPFlag("--localaddr", ch.Input.Localaddr).
		WithStringFlag("--timeout", strconv.FormatUint(uint64(ch.Input.Timeout), 10)).
		WithStringPFlag("--rtsp-transport", ch.Input.RTSPTransport)

	// --- Positional/outputs section: [--output [url] [output flags]]... ---
	for _, output := range ch.Outputs {
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
	return builder.BuildString()
}
