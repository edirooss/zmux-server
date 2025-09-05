// Package remuxcmd provides helpers for constructing argv slices and
// shell-safe command strings for the `remux` binary.
//
// Policy: emit everything unless the value is null.
// - Zero/false values are meaningful and must be emitted.
// - Optional strings (pointers) are only emitted if non-nil and non-empty.
package remuxcmd

import (
	"strconv"
	"strings"
)

// RemuxCommandBuilder builds argv and a shell-safe command string for `remux`.
type RemuxCommandBuilder struct {
	args []string
}

// NewRemuxCommandBuilder returns a builder pre-seeded with the binary name ("remux").
func NewRemuxCommandBuilder() *RemuxCommandBuilder {
	return &RemuxCommandBuilder{args: []string{"remux"}}
}

// WithInt64Flag appends a flag with a base-10 int64 value (always emitted).
func (b *RemuxCommandBuilder) WithInt64Flag(flag string, val int64) *RemuxCommandBuilder {
	b.args = append(b.args, flag, strconv.FormatInt(val, 10))
	return b
}

// WithIntFlag appends a flag with a base-10 int value (always emitted).
func (b *RemuxCommandBuilder) WithIntFlag(flag string, val int) *RemuxCommandBuilder {
	b.args = append(b.args, flag, strconv.FormatInt(int64(val), 10))
	return b
}

// WithUintFlag appends a flag with a base-10 uint value (always emitted).
func (b *RemuxCommandBuilder) WithUintFlag(flag string, val uint) *RemuxCommandBuilder {
	b.args = append(b.args, flag, strconv.FormatUint(uint64(val), 10))
	return b
}

// WithStringFlag appends a flag with a string value if non-empty.
// Empty string is invalid and will be skipped.
func (b *RemuxCommandBuilder) WithStringFlag(flag, val string) *RemuxCommandBuilder {
	if val == "" {
		return b
	}
	b.args = append(b.args, flag, val)
	return b
}

// WithStringPFlag appends a flag with a *string value if non-nil and non-empty.
func (b *RemuxCommandBuilder) WithStringPFlag(flag string, pVal *string) *RemuxCommandBuilder {
	if pVal == nil || *pVal == "" {
		return b
	}
	b.args = append(b.args, flag, *pVal)
	return b
}

// WithBoolFlag appends --flag=true or --flag=false (always emitted).
func (b *RemuxCommandBuilder) WithBoolFlag(flag string, val bool) *RemuxCommandBuilder {
	if val {
		b.args = append(b.args, flag+"=true")
	} else {
		b.args = append(b.args, flag+"=false")
	}
	return b
}

// WithString appends a positional string argument if non-empty.
// (Used for sentinels/positionals like --input, URLs, etc.)
func (b *RemuxCommandBuilder) WithString(arg string) *RemuxCommandBuilder {
	if arg != "" {
		b.args = append(b.args, arg)
	}
	return b
}

// WithStringP appends a positional *string argument if non-nil and non-empty.
func (b *RemuxCommandBuilder) WithStringP(pArg *string) *RemuxCommandBuilder {
	if pArg != nil && *pArg != "" {
		b.args = append(b.args, *pArg)
	}
	return b
}

// BuildArgs returns a defensive copy of the constructed argv slice.
func (b *RemuxCommandBuilder) BuildArgs() []string {
	out := make([]string, len(b.args))
	copy(out, b.args)
	return out
}

// BuildString returns a single shell-safe command string, quoting each arg.
// Safe for systemd ExecStart and POSIX shells.
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
