package main

import (
	"fmt"
	"os"
	"time"
)

// benchTiming emits per-phase elapsed times to stderr when KLAUDIUSH_BENCH_TIMING=1.
// Zero cost when disabled - a single os.Getenv check at construction.
type benchTiming struct {
	enabled bool
	start   time.Time
}

func newBenchTiming() *benchTiming {
	if os.Getenv("KLAUDIUSH_BENCH_TIMING") != "1" {
		return &benchTiming{}
	}

	return &benchTiming{enabled: true, start: time.Now()}
}

func (bt *benchTiming) mark(phase string) {
	if !bt.enabled {
		return
	}

	elapsed := time.Since(bt.start)
	fmt.Fprintf(os.Stderr, "{\"phase\":%q,\"elapsed_us\":%d}\n", phase, elapsed.Microseconds())
}
