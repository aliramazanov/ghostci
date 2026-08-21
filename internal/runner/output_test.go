package runner

import (
	"strings"
	"sync"
	"testing"
)

func TestBoundedBufferUnderLimit(t *testing.T) {
	t.Parallel()
	b := newBoundedBuffer(100)
	b.Write([]byte("hello world"))
	if got := b.String(); got != "hello world" {
		t.Errorf("got %q", got)
	}
	if strings.Contains(b.String(), "elided") {
		t.Error("should not elide when under the limit")
	}
}

func TestBoundedBufferKeepsBothEnds(t *testing.T) {
	t.Parallel()
	b := newBoundedBuffer(10)
	b.Write([]byte("HEAD______"))
	b.Write([]byte(strings.Repeat("x", 5000)))
	b.Write([]byte("______TAIL"))

	got := b.String()
	if !strings.HasPrefix(got, "HEAD______") {
		t.Errorf("head lost: %q", got[:min(40, len(got))])
	}
	if !strings.HasSuffix(got, "______TAIL") {
		t.Errorf("tail lost: %q", got[max(0, len(got)-40):])
	}
	if !strings.Contains(got, "elided") {
		t.Error("elision not marked")
	}
	if len(got) > 200 {
		t.Errorf("buffer grew to %d bytes despite a limit of 10", len(got))
	}
}

func TestBoundedBufferSingleHugeWrite(t *testing.T) {
	t.Parallel()
	b := newBoundedBuffer(16)
	b.Write([]byte(strings.Repeat("a", 100) + "ENDMARKER"))
	got := b.String()
	if !strings.Contains(got, "ENDMARKER") {
		t.Errorf("tail of a single oversized write lost: %q", got)
	}
	if len(got) > 200 {
		t.Errorf("buffer grew to %d bytes", len(got))
	}
}

func TestBoundedBufferConcurrentWrites(t *testing.T) {
	t.Parallel()
	b := newBoundedBuffer(1024)
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				b.Write([]byte("concurrent-write "))
			}
		}()
	}
	wg.Wait()
	if b.total != 50*100*len("concurrent-write ") {
		t.Errorf("lost writes: total = %d", b.total)
	}
}

func TestBoundedBufferNoFalseElision(t *testing.T) {
	t.Parallel()
	b := newBoundedBuffer(8)
	b.Write([]byte("AAAAAAAA"))
	b.Write([]byte("BBBBBBBB"))

	got := b.String()
	if strings.Contains(got, "elided") {
		t.Errorf("nothing was dropped but output claims elision: %q", got)
	}
	if got != "AAAAAAAABBBBBBBB" {
		t.Errorf("content altered: %q", got)
	}
}

func TestBoundedBufferElidesWhenItActuallyDrops(t *testing.T) {
	t.Parallel()
	b := newBoundedBuffer(4)
	b.Write([]byte("AAAA"))
	b.Write([]byte("XXXXXXXXXXXX"))
	b.Write([]byte("BBBB"))

	got := b.String()
	if !strings.Contains(got, "elided") {
		t.Errorf("bytes were dropped but no elision reported: %q", got)
	}
	if strings.Contains(got, "0 bytes elided") {
		t.Errorf("elision count should be positive: %q", got)
	}
}
