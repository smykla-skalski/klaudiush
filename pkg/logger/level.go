package logger

import "log/slog"

//go:generate enumer -type=Level -trimprefix=Level -transform=upper -json -text -yaml -sql
//go:generate go run github.com/smykla-labs/klaudiush/tools/enumerfix level_enumer.go

// Level represents the log level.
type Level int

const (
	// LevelDebug represents debug-level logging (most verbose).
	LevelDebug Level = iota

	// LevelInfo represents info-level logging (standard verbosity).
	LevelInfo

	// LevelError represents error-level logging (least verbose).
	LevelError
)

// ToSlogLevel converts Level to slog.Level.
func (l Level) ToSlogLevel() slog.Level {
	switch l {
	case LevelDebug:
		return slog.LevelDebug
	case LevelInfo:
		return slog.LevelInfo
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// LevelFromFlags determines the log level from debug and trace flags.
func LevelFromFlags(debug, trace bool) Level {
	switch {
	case trace:
		return LevelDebug
	case debug:
		return LevelInfo
	default:
		return LevelError
	}
}
