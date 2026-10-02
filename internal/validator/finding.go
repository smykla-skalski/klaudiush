package validator

import (
	"cmp"
	"slices"
	"strings"
)

// Finding is one actionable violation inside a validation result. A result
// can carry several: a commit message with a long title and a long body line
// is one blocked command with two findings, and the agent needs both repairs
// to fix them in one retry.
type Finding struct {
	// Reference identifies the violated rule.
	Reference Reference `json:"reference,omitempty"`

	// Location names where the violation is, e.g. "title" or "body line 4".
	Location string `json:"location,omitempty"`

	// Message states what is wrong.
	Message string `json:"message"`

	// Actual is the offending value as found. Leave it empty when the value
	// may be secret.
	Actual string `json:"actual,omitempty"`

	// Required is the effective configured requirement.
	Required string `json:"required,omitempty"`

	// Repair tells the agent what to change.
	Repair string `json:"repair,omitempty"`
}

// Code returns the finding's error code, or "" when it has no reference.
func (f Finding) Code() string {
	if f.Reference == "" {
		return ""
	}

	return f.Reference.Code()
}

// AddFinding appends structured findings to the result.
func (r *Result) AddFinding(findings ...Finding) *Result {
	r.Findings = append(r.Findings, findings...)

	return r
}

// MarkUnavailable records that the validator could not run, so the result
// describes a missing check rather than a violation in the input.
func (r *Result) MarkUnavailable() *Result {
	r.Unavailable = true

	return r
}

// MarkInspected records that the run checked the whole file as the tool left
// it, so the result can prove earlier findings in that file resolved.
func (r *Result) MarkInspected() *Result {
	r.Inspected = true

	return r
}

// SortFindings orders findings deterministically and drops exact duplicates.
// Findings keep the order of the given priority list first, then sort by code,
// location (numbers compared by value, so line 9 comes before line 10) and
// message.
func SortFindings(findings []Finding, priority []Reference) []Finding {
	if len(findings) == 0 {
		return nil
	}

	rank := make(map[Reference]int, len(priority))
	for i, ref := range priority {
		rank[ref] = i
	}

	rankOf := func(ref Reference) int {
		if i, ok := rank[ref]; ok {
			return i
		}

		return len(priority)
	}

	sorted := slices.Clone(findings)
	slices.SortStableFunc(sorted, func(a, b Finding) int {
		return cmp.Or(
			cmp.Compare(rankOf(a.Reference), rankOf(b.Reference)),
			cmp.Compare(a.Code(), b.Code()),
			compareNatural(a.Location, b.Location),
			cmp.Compare(a.Message, b.Message),
			cmp.Compare(a.Actual, b.Actual),
			cmp.Compare(a.Required, b.Required),
			cmp.Compare(a.Repair, b.Repair),
		)
	})

	return slices.Compact(sorted)
}

// compareNatural compares strings with digit runs ordered by numeric value.
func compareNatural(a, b string) int {
	for a != "" && b != "" {
		da, db := leadingDigits(a), leadingDigits(b)

		if da != "" && db != "" {
			if c := cmp.Or(
				cmp.Compare(len(strings.TrimLeft(da, "0")), len(strings.TrimLeft(db, "0"))),
				cmp.Compare(strings.TrimLeft(da, "0"), strings.TrimLeft(db, "0")),
			); c != 0 {
				return c
			}

			a, b = a[len(da):], b[len(db):]

			continue
		}

		if a[0] != b[0] {
			return cmp.Compare(a[0], b[0])
		}

		a, b = a[1:], b[1:]
	}

	return cmp.Compare(len(a), len(b))
}

func leadingDigits(s string) string {
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}

	return s[:end]
}
