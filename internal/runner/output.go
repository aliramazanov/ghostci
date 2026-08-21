package runner

import (
	"fmt"
	"sync"
)

type boundedBuffer struct {
	mu    sync.Mutex
	limit int
	head  []byte
	tail  []byte
	total int
}

func newBoundedBuffer(limit int) *boundedBuffer {
	return &boundedBuffer{limit: limit}
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	n := len(p)
	b.total += n

	if room := b.limit - len(b.head); room > 0 {
		take := min(room, len(p))
		b.head = append(b.head, p[:take]...)
		p = p[take:]
	}
	if len(p) == 0 {
		return n, nil
	}

	if len(p) >= b.limit {
		b.tail = append(b.tail[:0], p[len(p)-b.limit:]...)
		return n, nil
	}
	b.tail = append(b.tail, p...)
	if excess := len(b.tail) - b.limit; excess > 0 {
		b.tail = b.tail[excess:]
	}
	return n, nil
}

func (b *boundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	elided := b.total - len(b.head) - len(b.tail)
	if elided <= 0 {
		return string(b.head) + string(b.tail)
	}
	return fmt.Sprintf("%s\n... %d bytes elided ...\n%s", b.head, elided, b.tail)
}
