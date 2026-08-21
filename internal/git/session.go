package git

import (
	"strings"
	"sync"
)

type Session struct {
	Dir string

	root   func() (string, error)
	dirty  func() ([]string, error)
	blobs  func() (map[string]string, error)
	isRepo func() bool
	info   func() Info

	mu     sync.Mutex
	cached map[string]string
}

func NewSession(dir string) *Session {
	s := &Session{Dir: dir, cached: map[string]string{}}

	s.root = sync.OnceValues(func() (string, error) { return Root(dir) })
	s.dirty = sync.OnceValues(func() ([]string, error) { return Dirty(dir) })
	s.blobs = sync.OnceValues(func() (map[string]string, error) { return Blobs(dir) })
	s.isRepo = sync.OnceValue(func() bool { return IsRepo(dir) })
	s.info = sync.OnceValue(func() Info { return Discover(dir) })

	return s
}

func (s *Session) Root() (string, error)             { return s.root() }
func (s *Session) Dirty() ([]string, error)          { return s.dirty() }
func (s *Session) Blobs() (map[string]string, error) { return s.blobs() }
func (s *Session) IsRepo() bool                      { return s.isRepo() }
func (s *Session) Info() Info                        { return s.info() }

func (s *Session) Run(args ...string) (string, error) {
	key := join(args)

	s.mu.Lock()
	if got, ok := s.cached[key]; ok {
		s.mu.Unlock()

		return got, nil
	}
	s.mu.Unlock()

	out, err := Run(s.Dir, args...)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	s.cached[key] = out
	s.mu.Unlock()

	return out, nil
}

func (s *Session) Changed(base string) ([]string, error) {
	return Changed(s.Dir, base)
}

func (s *Session) SameCommit(a, b string) bool {
	ra, err1 := s.Run("rev-parse", a+"^{commit}")
	rb, err2 := s.Run("rev-parse", b+"^{commit}")

	return err1 == nil && err2 == nil && ra == rb
}

func join(args []string) string { return strings.Join(args, "\x00") }

func (s *Session) Verify(rev string) (string, error) { return s.Run(cmdVerify(rev)...) }

func (s *Session) Upstream() (string, error) { return s.Run(cmdUpstream...) }

func (s *Session) OriginHead() (string, error) { return s.Run(cmdOriginHead...) }

func (s *Session) MergeBase(a, b string) (string, error) { return s.Run(cmdMergeBase(a, b)...) }
