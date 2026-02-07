// Package config provides configuration schema types for klaudiush validators.
package config

import (
	"time"

	"github.com/cockroachdb/errors"
)

//go:generate enumer -type=Severity -trimprefix=Severity -transform=lower -json -text -yaml -sql
//go:generate go run github.com/smykla-labs/klaudiush/tools/enumerfix severity_enumer.go

var (
	// ErrInvalidSeverity is returned when an invalid severity value is provided.
	ErrInvalidSeverity = errors.New("invalid severity")

	// ErrNegativeDuration is returned when a negative duration is provided.
	ErrNegativeDuration = errors.New("duration must be non-negative")
)

// Severity represents the severity level of a validation failure.
type Severity int

const (
	// SeverityUnknown represents an unknown severity level.
	SeverityUnknown Severity = iota

	// SeverityError indicates a validation failure that blocks the operation.
	SeverityError

	// SeverityWarning indicates a validation failure that only warns without blocking.
	SeverityWarning
)

// ShouldBlock returns true if the severity should block the operation.
func (s Severity) ShouldBlock() bool {
	return s == SeverityError
}

// ParseSeverity parses a string into a Severity value.
func ParseSeverity(s string) (Severity, error) {
	severity, err := SeverityString(s)
	if err != nil {
		return SeverityUnknown,
			errors.Wrapf(
				ErrInvalidSeverity,
				"%q, must be %q or %q",
				s,
				SeverityError.String(),
				SeverityWarning.String(),
			)
	}

	return severity, nil
}

// Duration wraps time.Duration for TOML parsing.
type Duration time.Duration

// UnmarshalText implements encoding.TextUnmarshaler for TOML parsing.
func (d *Duration) UnmarshalText(text []byte) error {
	dur, err := time.ParseDuration(string(text))
	if err != nil {
		return errors.Wrap(err, "invalid duration")
	}

	if dur < 0 {
		return errors.Wrapf(ErrNegativeDuration, "got %s", dur)
	}

	*d = Duration(dur)

	return nil
}

// MarshalText implements encoding.TextMarshaler for TOML serialization.
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(time.Duration(d).String()), nil
}

// String returns the string representation of the duration.
func (d Duration) String() string {
	return time.Duration(d).String()
}

// ToDuration converts Duration to time.Duration.
func (d Duration) ToDuration() time.Duration {
	return time.Duration(d)
}
