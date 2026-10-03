package file

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestPEP723BlockLinearOnUnterminatedStarts(t *testing.T) {
	const n = 200000

	lines := strings.Split(strings.Repeat(pep723Start+"\n", n), "\n")
	topLevel := make([]bool, len(lines))

	for i := range n {
		topLevel[i] = true
	}

	start := time.Now()
	block := pep723Block(lines, topLevel)

	if slices.Contains(block, true) {
		t.Fatal("unterminated start lines must not form a block")
	}

	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("scan of %d start lines took %s, want linear time", n, elapsed)
	}
}
