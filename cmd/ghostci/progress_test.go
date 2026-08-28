package main

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestALongCheckSaysItIsStillGoing(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var mu sync.Mutex
	r := newRunningChecks(&out, &mu)

	start := time.Now()
	r.started("slow")
	r.since["slow"] = start

	r.announce(start.Add(slowAfter - time.Second))
	if out.Len() != 0 {
		t.Errorf("announced a check that had only been running a moment: %q", out.String())
	}

	r.announce(start.Add(slowAfter))
	if !strings.Contains(out.String(), "slow") {
		t.Errorf("a check past the threshold was not announced: %q", out.String())
	}

	before := out.Len()
	r.announce(start.Add(slowAfter + time.Second))
	if out.Len() != before {
		t.Errorf("announced the same check twice in a row: %q", out.String())
	}

	r.announce(start.Add(2*slowAfter + time.Second))
	if out.Len() == before {
		t.Error("a check running much longer stopped showing signs of life")
	}
}

func TestAFinishedCheckIsNotAnnounced(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var mu sync.Mutex
	r := newRunningChecks(&out, &mu)

	start := time.Now()
	r.started("done")
	r.since["done"] = start

	mu.Lock()
	r.finished("done")
	mu.Unlock()

	r.announce(start.Add(10 * slowAfter))

	if out.Len() != 0 {
		t.Errorf("announced a check that had already finished: %q", out.String())
	}
}
