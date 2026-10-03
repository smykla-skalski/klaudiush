package metrics

import (
	"cmp"
	"maps"
	"math"
	"slices"
)

const microsPerMilli = 1000.0

const p95 = 0.95

const p50 = 0.5

// recurringReports is the report count that makes a violation recurring.
const recurringReports = 2

type eventAcc struct {
	stats  EventStats
	micros []int64
}

// codeKey names a finding's code, or its validator when it has none.
func codeKey(f *Finding) string {
	if f.Code != "" {
		return f.Code
	}

	return "(" + f.Validator + ")"
}

func codeEntry(codes map[string]*CodeStats, key string) *CodeStats {
	entry := codes[key]
	if entry == nil {
		entry = &CodeStats{Code: key}
		codes[key] = entry
	}

	return entry
}

func countFinding(
	codes map[string]*CodeStats,
	unavailable map[[2]string]*UnavailableStats,
	f *Finding,
) {
	entry := codeEntry(codes, codeKey(f))
	entry.Reports++
	entry.Outcomes.add(f.Class)

	if f.Unavailable == "" {
		return
	}

	key := [2]string{f.Unavailable, f.Validator}

	stats := unavailable[key]
	if stats == nil {
		stats = &UnavailableStats{Reason: f.Unavailable, Validator: f.Validator}
		unavailable[key] = stats
	}

	stats.Count++

	if f.Class == ClassPrevented || f.Class == ClassHeld {
		stats.Blocked++
	}
}

func summarizeLatency(micros []int64) Latency {
	if len(micros) == 0 {
		return Latency{}
	}

	sorted := slices.Clone(micros)
	slices.Sort(sorted)

	return Latency{
		Count: len(sorted),
		P50:   millis(rank(sorted, p50)),
		P95:   millis(rank(sorted, p95)),
		Max:   millis(sorted[len(sorted)-1]),
	}
}

// rank returns the nearest-rank percentile of sorted values.
func rank(sorted []int64, percentile float64) int64 {
	idx := int(math.Ceil(percentile*float64(len(sorted)))) - 1

	return sorted[min(max(idx, 0), len(sorted)-1)]
}

func millis(micros int64) float64 {
	return float64(micros) / microsPerMilli
}

func eventStats(events map[string]*eventAcc) []EventStats {
	out := make([]EventStats, 0, len(events))

	for _, acc := range events {
		acc.stats.Latency = summarizeLatency(acc.micros)
		out = append(out, acc.stats)
	}

	slices.SortFunc(out, func(a, b EventStats) int {
		return cmp.Or(cmp.Compare(a.Provider, b.Provider), cmp.Compare(a.Event, b.Event))
	})

	return out
}

func codeStats(codes map[string]*CodeStats) []CodeStats {
	out := make([]CodeStats, 0, len(codes))
	for _, entry := range codes {
		out = append(out, *entry)
	}

	slices.SortFunc(out, func(a, b CodeStats) int {
		return cmp.Or(cmp.Compare(b.Reports, a.Reports), cmp.Compare(a.Code, b.Code))
	})

	return out
}

func unavailableStats(stats map[[2]string]*UnavailableStats) []UnavailableStats {
	out := make([]UnavailableStats, 0, len(stats))
	for _, entry := range stats {
		out = append(out, *entry)
	}

	slices.SortFunc(out, func(a, b UnavailableStats) int {
		return cmp.Or(
			cmp.Compare(b.Count, a.Count),
			cmp.Compare(a.Reason, b.Reason),
			cmp.Compare(a.Validator, b.Validator),
		)
	})

	return out
}

func validatorStats(validators map[string][]int64) []ValidatorStats {
	out := make([]ValidatorStats, 0, len(validators))

	for _, name := range slices.Sorted(maps.Keys(validators)) {
		out = append(out, ValidatorStats{
			Validator: name,
			Latency:   summarizeLatency(validators[name]),
		})
	}

	slices.SortStableFunc(out, func(a, b ValidatorStats) int {
		return cmp.Compare(b.Latency.P95, a.Latency.P95)
	})

	return out
}

// violationKey identifies a violation within a session.
type violationKey struct {
	validator string
	resource  string
	code      string
}

type openViolation struct {
	reports int
	gate    bool
}

// repairTracker replays records in time order and follows each violation
// until a check stops reporting it.
type repairTracker struct {
	stats           *RepairStats
	codes           map[string]*CodeStats
	open            map[string]map[violationKey]*openViolation
	retriesToRepair int
}

func newRepairTracker(stats *RepairStats, codes map[string]*CodeStats) *repairTracker {
	return &repairTracker{
		stats: stats,
		codes: codes,
		open:  map[string]map[violationKey]*openViolation{},
	}
}

func (t *repairTracker) observe(rec *Record) {
	if rec.Session == "" {
		for i := range rec.Findings {
			if rec.Findings[i].Violation {
				t.stats.Uncorrelated++
			}
		}

		return
	}

	session := t.open[rec.Session]
	if session == nil {
		session = map[violationKey]*openViolation{}
		t.open[rec.Session] = session
	}

	t.closeExcepted(session, rec)
	reported := t.report(session, rec)
	t.closeChecked(session, rec, reported)

	if rec.Gate && len(reported) == 0 && cleanGate(rec.Outcome) {
		for key, item := range session {
			if item.gate {
				t.repaired(session, key, item)
			}
		}
	}

	if len(session) == 0 {
		delete(t.open, rec.Session)
	}
}

func cleanGate(outcome Class) bool {
	return outcome == ClassPassed || outcome == ClassWarned || outcome == ClassExcepted
}

func (t *repairTracker) closeExcepted(session map[violationKey]*openViolation, rec *Record) {
	for i := range rec.Findings {
		f := &rec.Findings[i]
		if f.Class != ClassExcepted {
			continue
		}

		key := violationKey{validator: f.Validator, resource: f.Resource, code: codeKey(f)}
		if _, ok := session[key]; !ok {
			continue
		}

		delete(session, key)

		t.stats.Exceptions++
		codeEntry(t.codes, key.code).Exceptions++
	}
}

// report opens or repeats each violation of rec, once per record.
func (t *repairTracker) report(
	session map[violationKey]*openViolation,
	rec *Record,
) map[violationKey]bool {
	reported := map[violationKey]bool{}

	for i := range rec.Findings {
		f := &rec.Findings[i]
		if !f.Violation {
			continue
		}

		key := violationKey{validator: f.Validator, resource: f.Resource, code: codeKey(f)}
		if reported[key] {
			continue
		}

		reported[key] = true
		entry := codeEntry(t.codes, key.code)

		item := session[key]
		if item == nil {
			session[key] = &openViolation{reports: 1, gate: rec.Gate}
			t.stats.Violations++
			entry.Violations++

			continue
		}

		item.reports++
		item.gate = item.gate || rec.Gate
		t.stats.Retries++
		entry.Retries++

		if item.reports == recurringReports {
			t.stats.Recurring++
			entry.Recurring++
		}
	}

	return reported
}

// closeChecked repairs open violations whose validator checked their
// resource in rec without reporting them.
func (t *repairTracker) closeChecked(
	session map[violationKey]*openViolation,
	rec *Record,
	reported map[violationKey]bool,
) {
	checked := map[[2]string]bool{}
	for _, name := range rec.Checked {
		checked[[2]string{name, rec.Resource}] = true
	}

	for _, check := range rec.Other {
		checked[[2]string{check.Validator, check.Resource}] = true
	}

	for key, item := range session {
		if checked[[2]string{key.validator, key.resource}] && !reported[key] {
			t.repaired(session, key, item)
		}
	}
}

func (t *repairTracker) repaired(
	session map[violationKey]*openViolation,
	key violationKey,
	item *openViolation,
) {
	delete(session, key)

	entry := codeEntry(t.codes, key.code)
	t.stats.Repaired++
	entry.Repaired++

	if item.reports == 1 {
		t.stats.FirstTry++
		entry.FirstTry++
	}

	t.retriesToRepair += item.reports - 1
}

// finish counts what is still open as unresolved.
func (t *repairTracker) finish() {
	for _, session := range t.open {
		for key := range session {
			t.stats.Unresolved++
			codeEntry(t.codes, key.code).Unresolved++
		}
	}
}
