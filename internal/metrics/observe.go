package metrics

import (
	"slices"
	"time"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

// Observation is what one hook did, as the caller saw it when it wrote the
// response. Errors are the findings the response was built from; Stopped is
// whether that response told the harness to stop something (see
// hookresponse.Stops). Released marks a completion gate that let the turn
// end over unresolved findings, Skipped a hook the bypass policy did not
// validate, and Filtered a tool selection that withheld tools.
type Observation struct {
	Context  *hook.Context
	Errors   []*dispatcher.ValidationError
	Checks   []dispatcher.Check
	Timings  []dispatcher.Timing
	Stopped  bool
	Released bool
	Skipped  bool
	Filtered bool

	// ReleasedFindings marks, by index into Errors, the findings a released
	// completion gate turned from blocking into warnings. Nil means all.
	ReleasedFindings []bool
	Elapsed          time.Duration
	Time             time.Time
}

// hasher keys sessions and resources without storing them.
type hasher func(parts ...string) string

// build turns an observation into the record stored on disk.
func build(obs *Observation, hash hasher) *Record {
	hookCtx := obs.Context
	if hookCtx == nil {
		hookCtx = &hook.Context{}
	}

	rec := &Record{
		Time:     obs.Time.UTC(),
		Provider: token(string(hookCtx.Provider)),
		Event:    eventName(hookCtx),
		Micros:   obs.Elapsed.Microseconds(),
		Gate: hook.IsCompletionGate(
			hookCtx.Provider,
			hookCtx.Event,
			hookCtx.RawEventName,
		),
	}

	if rec.Provider == "" {
		rec.Provider = string(hook.ProviderClaude)
	}

	if hookCtx.SessionID != "" {
		rec.Session = hash("session", string(hookCtx.Provider), hookCtx.SessionID)
	}

	if hookCtx.AgentID != "" {
		rec.Agent = agentKey(hookCtx, hash)
	}

	// A subagent's stop checks only that subagent's findings, so it can
	// only show those repaired. Without an agent ID it checks none.
	if rec.Gate && hookCtx.Event == hook.CanonicalEventSubagentStop {
		rec.Scope = agentKey(hookCtx, hash)
	}

	if resource := hookCtx.Resource(); resource != "" {
		rec.Resource = hash("resource", resource)
	}

	all := findings(hookCtx, obs, rec, hash)
	rec.Outcome = outcome(obs, all)

	if len(all) > maxFindings {
		slices.SortStableFunc(all, func(a, b Finding) int {
			return findingRank(a) - findingRank(b)
		})
	}

	rec.Findings = all[:min(len(all), maxFindings)]
	rec.Truncated = len(all) > maxFindings

	if rec.Session != "" {
		rec.Checked, rec.Other = checks(obs.Checks, rec.Resource, hash)
	}

	rec.Timings = timings(obs.Timings)

	return rec
}

// agentKey keys the hook's subagent within its session.
func agentKey(hookCtx *hook.Context, hash hasher) string {
	return hash("agent", string(hookCtx.Provider), hookCtx.SessionID, hookCtx.AgentID)
}

// eventName is the native event the hook received, as a short token.
func eventName(hookCtx *hook.Context) string {
	if name := token(hookCtx.EventName()); name != "" {
		return name
	}

	if hookCtx.Event != hook.CanonicalEventUnknown {
		return token(string(hookCtx.Event))
	}

	return "unknown"
}

// findings classifies every error. The hook's outcome is taken over all of
// them; the record keeps the first maxFindings and marks itself truncated.
func findings(
	hookCtx *hook.Context,
	obs *Observation,
	rec *Record,
	hash hasher,
) []Finding {
	if len(obs.Errors) == 0 {
		return nil
	}

	afterTool := hookCtx.IsAfterTool()
	out := make([]Finding, 0, len(obs.Errors))

	for i, verr := range obs.Errors {
		if verr == nil {
			continue
		}

		released := obs.Released && rec.Gate &&
			(obs.ReleasedFindings == nil || i < len(obs.ReleasedFindings) && obs.ReleasedFindings[i])

		f := Finding{
			Validator: token(verr.Validator),
			Class:     findingClass(verr, obs, rec.Gate, afterTool, released),
			Violation: !verr.Bypassed && !verr.Unavailable &&
				(verr.ShouldBlock || afterTool || released),
		}

		if verr.Resource != "" {
			f.Resource = hash("resource", verr.Resource)
		}

		if verr.Unavailable {
			f.Unavailable = token(string(verr.UnavailableReason))
			if f.Unavailable == "" {
				f.Unavailable = "error"
			}
		}

		for _, code := range errorCodes(verr) {
			f.Code = code
			out = append(out, f)
		}
	}

	return out
}

// errorCodes lists the distinct codes of an error's structured findings, in
// order, or its header code when it has none: one error can carry several
// violations, each followed on its own.
func errorCodes(verr *dispatcher.ValidationError) []string {
	var codes []string

	for _, finding := range verr.Findings {
		if code := token(finding.Code()); code != "" && !slices.Contains(codes, code) {
			codes = append(codes, code)
		}
	}

	if len(codes) == 0 {
		return []string{token(verr.Reference.Code())}
	}

	return codes
}

// findingClass is what one finding did. A blocking finding prevents the
// action only when the response stopped it before it happened; after a tool
// ran, or on an event the provider cannot stop, it is advisory.
func findingClass(
	verr *dispatcher.ValidationError,
	obs *Observation,
	gate, afterTool, released bool,
) Class {
	switch {
	case verr.Bypassed:
		return ClassExcepted
	case verr.ShouldBlock && obs.Stopped && !afterTool && gate:
		return ClassHeld
	case verr.ShouldBlock && obs.Stopped && !afterTool:
		return ClassPrevented
	case verr.Unavailable:
		return ClassUnavailable
	case released:
		return ClassReleased
	case verr.ShouldBlock || afterTool:
		return ClassAdvisory
	default:
		return ClassWarned
	}
}

// findingRank orders findings for the capped list: violations first, then
// by class strength, so the findings behind the outcome are kept.
func findingRank(f Finding) int {
	rank := slices.Index(classOrder, f.Class)
	if rank < 0 {
		rank = len(classOrder)
	}

	if !f.Violation {
		rank += len(classOrder) + 1
	}

	return rank
}

// outcome is the strongest class of the findings. Skipped applies only when
// nothing was reported: checks that run whatever the bypass policy says,
// such as the completion gates, still count for what they did.
func outcome(obs *Observation, found []Finding) Class {
	for _, class := range classOrder {
		if slices.ContainsFunc(found, func(f Finding) bool { return f.Class == class }) {
			return class
		}
	}

	switch {
	case obs.Skipped:
		return ClassSkipped
	case obs.Filtered:
		return ClassAdvisory
	default:
		return ClassPassed
	}
}

// checks lists the validators that ran to completion: by name on the hook's
// own resource, with the resource key elsewhere.
func checks(all []dispatcher.Check, primary string, hash hasher) ([]string, []Check) {
	var (
		names []string
		other []Check
	)

	for _, check := range all {
		name := token(check.Validator)
		resource := hash("resource", check.Resource)

		if resource == primary {
			if !slices.Contains(names, name) && len(names) < maxChecks {
				names = append(names, name)
			}

			continue
		}

		entry := Check{Validator: name, Resource: resource}
		if !slices.Contains(other, entry) && len(other) < maxChecks {
			other = append(other, entry)
		}
	}

	return names, other
}

// timings sums each validator's run time in microseconds.
func timings(all []dispatcher.Timing) map[string]int64 {
	if len(all) == 0 {
		return nil
	}

	out := make(map[string]int64, min(len(all), maxTimings))

	for _, timing := range all {
		name := token(timing.Validator)
		if _, ok := out[name]; !ok && len(out) == maxTimings {
			continue
		}

		out[name] += timing.Elapsed.Microseconds()
	}

	return out
}
