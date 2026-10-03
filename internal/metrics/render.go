package metrics

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

const (
	tabMinWidth = 0
	tabWidth    = 4
	tabPadding  = 2

	maxRenderedCodes      = 25
	maxRenderedValidators = 15
	percent               = 100
)

// Render writes the report as text tables.
func Render(w io.Writer, report *Report) error {
	pw := &printer{w: w}

	pw.printf("Enforcement outcomes %s to %s (%d hooks",
		report.Since.Local().Format(time.DateTime),
		report.Until.Local().Format(time.DateTime),
		report.Records,
	)

	if report.SkippedLines > 0 {
		pw.printf(", %d unreadable lines skipped", report.SkippedLines)
	}

	pw.printf(")\n")

	if report.Records == 0 {
		pw.printf("\nNo hooks recorded in this window.\n")

		return pw.err
	}

	o := report.Outcomes
	pw.printf("\nEnforced: %d (prevented %d, completion gate held %d)\n",
		o.Enforced(), o.Prevented, o.Held)
	pw.printf("Not enforced: advisory %d, gate released %d, warned %d, "+
		"exception accepted %d, unavailable %d, validation skipped %d\n",
		o.Advisory, o.Released, o.Warned, o.Excepted, o.Unavailable, o.Skipped)
	pw.printf("Passed: %d\n", o.Passed)
	pw.printf("Hook latency: p50 %.1fms, p95 %.1fms, max %.1fms\n",
		report.Latency.P50, report.Latency.P95, report.Latency.Max)

	renderEvents(pw, report.Events)
	renderRepairs(pw, &report.Repairs)
	renderCodes(pw, report.Codes)
	renderUnavailable(pw, report.Unavailable)
	renderValidators(pw, report.Validators)

	return pw.err
}

type printer struct {
	w   io.Writer
	err error
}

func (p *printer) printf(format string, args ...any) {
	if p.err != nil {
		return
	}

	_, p.err = fmt.Fprintf(p.w, format, args...)
}

func (p *printer) table(header string, rows [][]string) {
	if p.err != nil {
		return
	}

	tw := tabwriter.NewWriter(p.w, tabMinWidth, tabWidth, tabPadding, ' ', 0)

	_, p.err = fmt.Fprintln(tw, header)

	for _, row := range rows {
		if p.err != nil {
			return
		}

		_, p.err = fmt.Fprintln(tw, strings.Join(row, "\t"))
	}

	if p.err == nil {
		p.err = tw.Flush()
	}
}

func renderEvents(pw *printer, events []EventStats) {
	pw.printf("\nBy provider and event:\n")

	rows := make([][]string, 0, len(events))
	for _, e := range events {
		o := e.Outcomes
		rows = append(rows, []string{
			e.Provider, e.Event, itoa(e.Invocations),
			itoa(o.Prevented), itoa(o.Held), itoa(o.Advisory), itoa(o.Released),
			itoa(o.Warned), itoa(o.Excepted), itoa(o.Unavailable), itoa(o.Passed),
			itoa(o.Skipped), ms(e.Latency.P50), ms(e.Latency.P95),
		})
	}

	pw.table("PROVIDER\tEVENT\tHOOKS\tPREVENTED\tHELD\tADVISORY\tRELEASED\tWARNED\t"+
		"EXCEPTED\tUNAVAILABLE\tPASSED\tSKIPPED\tP50\tP95", rows)
}

func renderRepairs(pw *printer, r *RepairStats) {
	pw.printf("\nRepairs: %d violations, %d repaired (%d on the first try, "+
		"mean %.2f retries), %d closed by exception, %d unresolved\n",
		r.Violations, r.Repaired, r.FirstTry, r.MeanRetries, r.Exceptions, r.Unresolved)
	pw.printf("Retries: %d repeated reports, %d recurring violations",
		r.Retries, r.Recurring)

	if r.Uncorrelated > 0 {
		pw.printf(", %d reports without a session", r.Uncorrelated)
	}

	pw.printf("\n")
}

func renderCodes(pw *printer, codes []CodeStats) {
	if len(codes) == 0 {
		return
	}

	pw.printf("\nBy code:\n")

	rows := make([][]string, 0, min(len(codes), maxRenderedCodes))
	for _, c := range codes[:min(len(codes), maxRenderedCodes)] {
		o := c.Outcomes
		rows = append(rows, []string{
			c.Code, itoa(c.Reports), itoa(o.Enforced()), itoa(o.Advisory),
			itoa(o.Released), itoa(o.Warned), itoa(o.Excepted), itoa(o.Unavailable),
			itoa(c.Repaired), itoa(c.Retries), itoa(c.Recurring), itoa(c.Unresolved),
			exceptionRate(c),
		})
	}

	pw.table("CODE\tREPORTS\tENFORCED\tADVISORY\tRELEASED\tWARNED\tEXCEPTED\tUNAVAILABLE\t"+
		"REPAIRED\tRETRIES\tRECURRING\tUNRESOLVED\tEXCEPTION RATE", rows)
}

// exceptionRate is the share of a code's reports that were waived, the
// closest local signal of false positives.
func exceptionRate(c CodeStats) string {
	if c.Reports == 0 {
		return "-"
	}

	return fmt.Sprintf("%.0f%%", float64(c.Outcomes.Excepted)*percent/float64(c.Reports))
}

func renderUnavailable(pw *printer, stats []UnavailableStats) {
	if len(stats) == 0 {
		return
	}

	pw.printf("\nUnavailable checks:\n")

	rows := make([][]string, 0, len(stats))
	for _, s := range stats {
		rows = append(rows, []string{s.Reason, s.Validator, itoa(s.Count), itoa(s.Blocked)})
	}

	pw.table("REASON\tVALIDATOR\tCOUNT\tBLOCKED", rows)
}

func renderValidators(pw *printer, validators []ValidatorStats) {
	if len(validators) == 0 {
		return
	}

	pw.printf("\nSlowest validators:\n")

	rows := make([][]string, 0, min(len(validators), maxRenderedValidators))
	for _, v := range validators[:min(len(validators), maxRenderedValidators)] {
		rows = append(rows, []string{
			v.Validator, itoa(v.Latency.Count),
			ms(v.Latency.P50), ms(v.Latency.P95), ms(v.Latency.Max),
		})
	}

	pw.table("VALIDATOR\tRUNS\tP50\tP95\tMAX", rows)
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func ms(v float64) string {
	return fmt.Sprintf("%.1fms", v)
}
