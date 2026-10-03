// Package metrics records local enforcement outcome metrics and reports
// them: what each hook response actually did (prevented, kept a completion
// gate shut, only advised, warned, accepted an exception, or could not
// validate), how violations were repaired, which checks could not run, and
// how long hooks and validators took.
//
// One JSON line per hook is appended to a size-capped log. Records hold no
// command, message, file path or session ID: sessions and resources are
// keyed by a salted hash that never leaves the state directory, and
// validator names, codes and event names are reduced to short tokens.
package metrics

import (
	"strings"
	"time"
)

// Class is what a hook, or one finding in it, did to the action.
type Class string

// Classes, strongest first. A hook takes the strongest class of its findings.
const (
	ClassPrevented   Class = "prevented"
	ClassHeld        Class = "held"
	ClassReleased    Class = "released"
	ClassAdvisory    Class = "advisory"
	ClassUnavailable Class = "unavailable"
	ClassExcepted    Class = "excepted"
	ClassWarned      Class = "warned"
	ClassPassed      Class = "passed"
	ClassSkipped     Class = "skipped"
)

// classOrder ranks classes for picking a hook's outcome.
var classOrder = []Class{
	ClassPrevented,
	ClassHeld,
	ClassReleased,
	ClassAdvisory,
	ClassUnavailable,
	ClassExcepted,
	ClassWarned,
}

// Record is one hook invocation as stored on disk.
type Record struct {
	Time      time.Time        `json:"t"`
	Provider  string           `json:"p"`
	Event     string           `json:"e"`
	Session   string           `json:"s,omitempty"`
	Resource  string           `json:"r,omitempty"`
	Outcome   Class            `json:"o"`
	Micros    int64            `json:"us"`
	Gate      bool             `json:"g,omitempty"`
	Truncated bool             `json:"tr,omitempty"`
	Findings  []Finding        `json:"f,omitempty"`
	Checked   []string         `json:"c,omitempty"`
	Other     []Check          `json:"cx,omitempty"`
	Timings   map[string]int64 `json:"vt,omitempty"`
}

// Finding is one reported error of a hook. Violation marks findings that ask
// the agent to change something: blocking ones, and every unwaived finding
// after a tool ran.
type Finding struct {
	Code        string `json:"c,omitempty"`
	Validator   string `json:"v"`
	Resource    string `json:"r,omitempty"`
	Class       Class  `json:"k"`
	Violation   bool   `json:"x,omitempty"`
	Unavailable string `json:"u,omitempty"`
}

// Check is a validator that ran to completion on a resource other than the
// hook's own.
type Check struct {
	Validator string `json:"v"`
	Resource  string `json:"r"`
}

// Record list bounds keep one line small whatever a hook reports.
const (
	maxFindings = 32
	maxChecks   = 64
	maxTimings  = 64
	maxToken    = 64
)

// token reduces a name to at most maxToken characters of a safe alphabet,
// so a crafted validator or event name cannot carry arbitrary text into
// the log.
func token(s string) string {
	s = strings.TrimPrefix(s, "validate-")

	var b strings.Builder

	for _, r := range s {
		if b.Len() >= maxToken {
			break
		}

		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '_', r == '-', r == ':':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}

	return b.String()
}
