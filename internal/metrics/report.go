package metrics

import (
	"slices"
	"strings"
	"time"
)

// Filter narrows a report.
type Filter struct {
	Provider string
	Event    string
}

func (f Filter) matches(rec *Record) bool {
	return (f.Provider == "" || strings.EqualFold(f.Provider, rec.Provider)) &&
		(f.Event == "" || strings.EqualFold(f.Event, rec.Event))
}

// Report summarizes records.
type Report struct {
	Since        time.Time          `json:"since"`
	Until        time.Time          `json:"until"`
	Records      int                `json:"records"`
	SkippedLines int                `json:"skipped_lines,omitempty"`
	Outcomes     Outcomes           `json:"outcomes"`
	Latency      Latency            `json:"latency"`
	Events       []EventStats       `json:"events"`
	Codes        []CodeStats        `json:"codes"`
	Repairs      RepairStats        `json:"repairs"`
	Unavailable  []UnavailableStats `json:"unavailable"`
	Validators   []ValidatorStats   `json:"validators"`
}

// Outcomes counts hooks or findings by class. Prevented counts only actions
// a response stopped before they happened; findings after a tool ran are
// advisory however they are worded.
type Outcomes struct {
	Prevented   int `json:"prevented"`
	Held        int `json:"held"`
	Released    int `json:"released"`
	Advisory    int `json:"advisory"`
	Unavailable int `json:"unavailable"`
	Excepted    int `json:"excepted"`
	Warned      int `json:"warned"`
	Passed      int `json:"passed"`
	Skipped     int `json:"skipped"`
}

func (o *Outcomes) add(class Class) {
	switch class {
	case ClassPrevented:
		o.Prevented++
	case ClassHeld:
		o.Held++
	case ClassReleased:
		o.Released++
	case ClassAdvisory:
		o.Advisory++
	case ClassUnavailable:
		o.Unavailable++
	case ClassExcepted:
		o.Excepted++
	case ClassWarned:
		o.Warned++
	case ClassPassed:
		o.Passed++
	case ClassSkipped:
		o.Skipped++
	}
}

// Enforced counts outcomes where the response stopped the action or kept a
// completion gate shut.
func (o Outcomes) Enforced() int {
	return o.Prevented + o.Held
}

// Latency summarizes durations in milliseconds (nearest rank).
type Latency struct {
	Count int     `json:"count"`
	P50   float64 `json:"p50_ms"`
	P95   float64 `json:"p95_ms"`
	Max   float64 `json:"max_ms"`
}

// EventStats are the hooks of one provider/event pair.
type EventStats struct {
	Provider    string   `json:"provider"`
	Event       string   `json:"event"`
	Invocations int      `json:"invocations"`
	Outcomes    Outcomes `json:"outcomes"`
	Latency     Latency  `json:"latency"`
}

// CodeStats are the findings of one error code.
type CodeStats struct {
	Code       string   `json:"code"`
	Reports    int      `json:"reports"`
	Outcomes   Outcomes `json:"outcomes"`
	Violations int      `json:"violations"`
	Repaired   int      `json:"repaired"`
	FirstTry   int      `json:"repaired_first_try"`
	Retries    int      `json:"retries"`
	Recurring  int      `json:"recurring"`
	Exceptions int      `json:"closed_by_exception"`
	Unresolved int      `json:"unresolved"`
}

// RepairStats follows each violation (one validator, resource and code in
// a session) from its first report until a check no longer reports it
// (repaired), an exception accepts it, or the window ends (unresolved).
// Retries counts reports of a violation that was already open; recurring
// counts violations reported more than once. Violations without a session
// cannot be followed and are counted as uncorrelated.
type RepairStats struct {
	Violations   int     `json:"violations"`
	Repaired     int     `json:"repaired"`
	FirstTry     int     `json:"repaired_first_try"`
	Retries      int     `json:"retries"`
	Recurring    int     `json:"recurring"`
	Exceptions   int     `json:"closed_by_exception"`
	Unresolved   int     `json:"unresolved"`
	Uncorrelated int     `json:"uncorrelated"`
	MeanRetries  float64 `json:"mean_retries_to_repair"`
}

// UnavailableStats counts checks that could not run, by reason and validator.
type UnavailableStats struct {
	Reason    string `json:"reason"`
	Validator string `json:"validator"`
	Count     int    `json:"count"`
	Blocked   int    `json:"blocked"`
}

// ValidatorStats is one validator's run time.
type ValidatorStats struct {
	Validator string  `json:"validator"`
	Latency   Latency `json:"latency"`
}

// Summarize builds a report from records between since and until.
func Summarize(records []Record, since, until time.Time, filter Filter) *Report {
	report := &Report{Since: since, Until: until}

	events := map[string]*eventAcc{}
	codes := map[string]*CodeStats{}
	unavailable := map[[2]string]*UnavailableStats{}
	validators := map[string][]int64{}
	tracker := newRepairTracker(&report.Repairs, codes)

	var hookMicros []int64

	sorted := slices.Clone(records)
	slices.SortStableFunc(sorted, func(a, b Record) int { return a.Time.Compare(b.Time) })

	for i := range sorted {
		rec := &sorted[i]
		if !filter.matches(rec) || rec.Time.After(until) {
			continue
		}

		report.Records++
		report.Outcomes.add(rec.Outcome)
		hookMicros = append(hookMicros, rec.Micros)

		event := events[rec.Provider+"\x00"+rec.Event]
		if event == nil {
			event = &eventAcc{stats: EventStats{Provider: rec.Provider, Event: rec.Event}}
			events[rec.Provider+"\x00"+rec.Event] = event
		}

		event.stats.Invocations++
		event.stats.Outcomes.add(rec.Outcome)
		event.micros = append(event.micros, rec.Micros)

		for j := range rec.Findings {
			countFinding(codes, unavailable, &rec.Findings[j])
		}

		for name, micros := range rec.Timings {
			validators[name] = append(validators[name], micros)
		}

		tracker.observe(rec)
	}

	tracker.finish()

	report.Latency = summarizeLatency(hookMicros)
	report.Events = eventStats(events)
	report.Codes = codeStats(codes)
	report.Unavailable = unavailableStats(unavailable)
	report.Validators = validatorStats(validators)

	if report.Repairs.Repaired > 0 {
		report.Repairs.MeanRetries = float64(tracker.retriesToRepair) /
			float64(report.Repairs.Repaired)
	}

	return report
}
