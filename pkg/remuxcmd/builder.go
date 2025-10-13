// Package remuxcmd builds canonical CLI invocations for the `remux` binary.
//
// Design:
//
//   - This layer is a pure "command construction" module: no execution, no I/O.
//     It normalizes CLI emission semantics and returns one of three canonical
//     projections of the same intent: argv (process argument vector), a shell-
//     quoted command string (for logging/systemd), or a prepared *exec.Cmd
//     (unstarted; caller configures stdio/env/cwd).
//
// Emission policy is deterministic and explicit:
//
//   - Numeric + boolean flags are ALWAYS emitted (including 0/false).
//   - Optional strings (pointers) are emitted only when non-nil AND non-empty.
//   - argv[0] is always the binary name ("remux"), mirroring POSIX/Go norms.
//
// API ergonomics:
//
//   - High-level conveniences: BuildArgv/BuildString/BuildCommand accept a
//     domain-level *channel.ZmuxChannel.
//   - Lower-level fluent Builder for composability and testing.
//   - This package deliberately owns CLI shape (flags/ordering/quoting) and
//     nothing else. Process lifecycle belongs in a higher layer.
//
// Usage:
//
//	argv := remuxcmd.BuildArgv(ch)      // []string{ "remux", "--id", ... }
//	s    := remuxcmd.BuildString(ch)    // "'remux' '--id' '42_7' ..."
//	cmd  := remuxcmd.BuildCommand(ch)   // *exec.Cmd (not started)
//
// Extensibility:
//
//	If CLI surface evolves, add more WithX methods and update FromChannel to
//	preserve order/semantics. Public conveniences remain stable.
package remuxcmd

import (
	"strconv"
	"strings"

	"github.com/edirooss/zmux-server/internal/domain/channel"
)

// Builder constructs argv and shell-safe command strings for `remux`.
//
// The Builder implements a fluent API; it is NOT concurrency-safe.
// Callers should treat a Builder as single-use, short-lived value objects.
//
// Invariants:
//   - argv[0] is always "remux".
//   - All With* methods are deterministic and order-preserving.
//   - BuildArgv returns a defensive copy.
type Builder struct {
	args []string // argv including binary name at index 0
}

// NewBuilder returns a Builder pre-seeded with the binary name ("remux").
//
// This is the lowest-level entrypoint for manual composition; most callers
// should prefer FromChannel + Build* conveniences.
func NewBuilder() *Builder {
	return &Builder{args: []string{"remux"}}
}

///////////////////////////////
// Fluent flag/arg builders. //
///////////////////////////////

// WithInt64Flag appends a flag with a base-10 int64 value (always emitted).
func (b *Builder) WithInt64Flag(flag string, val int64) *Builder {
	b.args = append(b.args, flag, strconv.FormatInt(val, 10))
	return b
}

// WithIntFlag appends a flag with a base-10 int value (always emitted).
func (b *Builder) WithIntFlag(flag string, val int) *Builder {
	b.args = append(b.args, flag, strconv.Itoa(val))
	return b
}

// WithUintFlag appends a flag with a base-10 uint value (always emitted).
func (b *Builder) WithUintFlag(flag string, val uint) *Builder {
	b.args = append(b.args, flag, strconv.FormatUint(uint64(val), 10))
	return b
}

// WithStringFlag appends a flag with a string value if non-empty.
// Empty string is considered invalid and skipped to avoid surprising empties.
func (b *Builder) WithStringFlag(flag, val string) *Builder {
	if val != "" {
		b.args = append(b.args, flag, val)
	}
	return b
}

// WithStringPFlag appends a flag with a *string value if non-nil and non-empty.
func (b *Builder) WithStringPFlag(flag string, pVal *string) *Builder {
	if pVal != nil && *pVal != "" {
		b.args = append(b.args, flag, *pVal)
	}
	return b
}

// WithBoolFlag appends --flag=true or --flag=false (always emitted).
// Always emitting booleans makes differences auditable and intent explicit.
func (b *Builder) WithBoolFlag(flag string, val bool) *Builder {
	if val {
		b.args = append(b.args, flag+"=true")
	} else {
		b.args = append(b.args, flag+"=false")
	}
	return b
}

// WithString appends a positional string argument if non-empty.
// Used for sentinels/positionals like --input, URLs, etc.
func (b *Builder) WithString(arg string) *Builder {
	if arg != "" {
		b.args = append(b.args, arg)
	}
	return b
}

// WithStringP appends a positional *string argument if non-nil and non-empty.
func (b *Builder) WithStringP(pArg *string) *Builder {
	if pArg != nil && *pArg != "" {
		b.args = append(b.args, *pArg)
	}
	return b
}

///////////////////////////
// Build output methods. //
///////////////////////////

// BuildArgv returns a defensive copy of the constructed argument vector.
//
// The first element (argv[0]) is always "remux". This mirrors POSIX/C main()
// and Go's exec.Command conventions and allows round-tripping to process APIs.
func (b *Builder) BuildArgv() []string {
	out := make([]string, len(b.args))
	copy(out, b.args)
	return out
}

// BuildString returns a single shell-quoted command string.
//
// Quoting strategy:
//   - Single-quote wrapping with inner single quotes escaped as:  ' -> '\”
//   - This is safe for POSIX shells and systemd ExecStart lines.
func (b *Builder) BuildString() string {
	quoted := make([]string, len(b.args))
	for i, a := range b.args {
		quoted[i] = shQuote(a)
	}
	return strings.Join(quoted, " ")
}

/////////////////////////////////
// High-level convenience API. //
/////////////////////////////////

// BuildArgv constructs the canonical argv for remux from a ZmuxChannel.
// Pure convenience over FromChannel(c).BuildArgv().
func BuildArgv(c *channel.ZmuxChannel) []string {
	return FromChannel(c).BuildArgv()
}

// BuildString constructs the canonical shell-quoted remux command string.
// Pure convenience over FromChannel(c).BuildString().
func BuildString(c *channel.ZmuxChannel) string {
	return FromChannel(c).BuildString()
}

//////////////////////
// Internal helpers //
//////////////////////

// shQuote returns a POSIX/systemd-safe single-quoted token.
//
// Empty strings become "”" to preserve round-trippability. This matches
// traditional /bin/sh semantics and prevents whitespace/glob expansion.
func shQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
