// services/remux_command_builder.go
package services

import (
	"strconv"
	"strings"

	models "github.com/edirooss/zmux-server/pkg/models/channel"
)

// RemuxCommandBuilder builds the final argv/command string for the `remux` binary.
// It omits zero-value fields and respects pflag boolean semantics:
//   - If a bool flag's default is true, only emit it when you need to turn it off: --flag=false
//   - If a bool flag's default is false, only emit it when you need to turn it on: --flag
type RemuxCommandBuilder struct {
	args []string
}

// NewRemuxCommandBuilder creates a new builder, pre-seeded with the binary name.
func NewRemuxCommandBuilder() *RemuxCommandBuilder {
	return &RemuxCommandBuilder{args: []string{"remux"}}
}

// WithString adds a string flag if val is non-empty (after trimming spaces).
func (b *RemuxCommandBuilder) WithString(flag, val string) *RemuxCommandBuilder {
	if strings.TrimSpace(val) != "" {
		b.args = append(b.args, flag, val)
	}
	return b
}

// WithInt adds a int flag.
func (b *RemuxCommandBuilder) WithInt(flag string, val int) *RemuxCommandBuilder {
	b.args = append(b.args, flag, strconv.FormatInt(int64(val), 10))
	return b
}

// WithUint adds a uint flag.
func (b *RemuxCommandBuilder) WithUint(flag string, val uint) *RemuxCommandBuilder {
	b.args = append(b.args, flag, strconv.FormatUint(uint64(val), 10))
	return b
}

// WithBoolDefault emits the flag only when val differs from its default.
// - def=true  & val=false → "--flag=false"
// - def=false & val=true  → "--flag"
func (b *RemuxCommandBuilder) WithBoolDefault(flag string, val, def bool) *RemuxCommandBuilder {
	if val == def {
		return b // omit, same as default
	}
	if def {
		// default true → explicitly disable
		b.args = append(b.args, flag+"=false")
		return b
	}
	// default false → enable by presence
	b.args = append(b.args, flag)
	return b
}

// BuildArgs returns the constructed argv slice.
func (b *RemuxCommandBuilder) BuildArgs() []string {
	out := make([]string, len(b.args))
	copy(out, b.args)
	return out
}

// BuildString returns a single shell-safe ExecStart string.
func (b *RemuxCommandBuilder) BuildString() string {
	quoted := make([]string, len(b.args))
	for i, a := range b.args {
		quoted[i] = shQuote(a)
	}
	return strings.Join(quoted, " ")
}

// shQuote wraps s in single quotes, escaping any internal single quotes.
// This is safe for systemd ExecStart and POSIX shells.
func shQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// BuildRemuxExecArgs maps models.ZmuxChannel → argv slice.
// ORDER MATCHES config.Config field order for maximum compatibility:
//
//	Top-level:        --id
//	Stream mapping:   --map-video --map-audio --map-data
//	Source:          --input-url --avioflags --probesize --analyzeduration --fflags
//	                  --max-delay --input-localaddr --timeout --rtsp-transport
//	Sink:            --output-url --output-localaddr --pkt-size
//
// Notes:
//   - Bool defaults are true for map-* flags; we only emit when disabling.
//   - Zero/empty values are omitted entirely (pflag won't receive "" values).
func BuildRemuxExecArgs(ch *models.ZmuxChannel) []string {
	builder := NewRemuxCommandBuilder()

	// --- Top-level ---
	builder.WithString("--id", strconv.FormatInt(ch.ID, 10))
	// (LogLevel is part of Config, not ZmuxChannel; omit here.)

	// --- Stream mapping (bool defaults true) ---
	builder.
		WithBoolDefault("--map-video", ch.MapVideo, true).
		WithBoolDefault("--map-audio", ch.MapAudio, true).
		WithBoolDefault("--map-data", ch.MapData, true)

	// --- Source (strings; omit if empty) ---
	builder.
		WithString("--input-url", ch.Source.InputURL).
		WithString("--avioflags", ch.Source.AVIOFlags).
		WithUint("--probesize", ch.Source.Probesize).
		WithUint("--analyzeduration", ch.Source.Analyzeduration).
		WithString("--fflags", ch.Source.FFlags).
		WithInt("--max-delay", ch.Source.MaxDelay).
		WithString("--input-localaddr", ch.Source.Localaddr).
		WithUint("--timeout", ch.Source.Timeout).
		WithString("--rtsp-transport", ch.Source.RTSPTransport)

	// --- Sink ---
	builder.
		WithString("--output-url", ch.Sink.OutputURL).
		WithString("--output-localaddr", ch.Sink.Localaddr).
		WithUint("--pkt-size", ch.Sink.PktSize)

	return builder.BuildArgs()
}

// BuildRemuxExecStart is a convenience wrapper that returns a shell-safe ExecStart string.
func BuildRemuxExecStart(ch *models.ZmuxChannel) string {
	return NewRemuxCommandBuilderFromArgs(BuildRemuxExecArgs(ch)).BuildString()
}

// NewRemuxCommandBuilderFromArgs seeds a builder with a prebuilt argv slice.
// The slice must include the binary name as args[0]. If empty, it seeds with "remux".
func NewRemuxCommandBuilderFromArgs(args []string) *RemuxCommandBuilder {
	if len(args) == 0 {
		return NewRemuxCommandBuilder()
	}
	// Ensure we keep a private copy
	cp := make([]string, len(args))
	copy(cp, args)
	return &RemuxCommandBuilder{args: cp}
}
