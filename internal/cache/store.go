package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Mode struct {
	Read  bool
	Write bool
}

func ReadWrite() Mode { return Mode{Read: true, Write: true} }

func Bypass() Mode { return Mode{Write: true} }

func Off() Mode { return Mode{} }

type Store struct {
	dir  string
	mode Mode

	mu     sync.Mutex
	latest map[string]record
}

type record struct {
	Name        string      `json:"name"`
	RecordedAt  time.Time   `json:"recorded_at"`
	DurationMS  int64       `json:"duration_ms"`
	Fingerprint Fingerprint `json:"fingerprint"`
}

func Open(root string, mode Mode) *Store {
	return &Store{dir: filepath.Join(root, ".ghostci", "cache"), mode: mode}
}

func (s *Store) Lookup(fp Fingerprint) (hit bool, previous Fingerprint) {
	if !s.mode.Read {
		return false, Fingerprint{}
	}

	key, err := fp.Key()

	if err != nil {
		return false, Fingerprint{}
	}

	body, err := os.ReadFile(filepath.Join(s.dir, key+".json"))

	if err != nil {
		return false, Fingerprint{}
	}

	var rec record

	if err := json.Unmarshal(body, &rec); err != nil {
		return false, Fingerprint{}
	}

	return true, rec.Fingerprint
}

func (s *Store) Record(name string, fp Fingerprint, took time.Duration) {
	if !s.mode.Write {
		return
	}

	key, err := fp.Key()

	if err != nil {
		return
	}

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return
	}

	body, err := json.MarshalIndent(record{
		Name:        name,
		RecordedAt:  time.Now(),
		DurationMS:  took.Milliseconds(),
		Fingerprint: fp,
	}, "", "  ")

	if err != nil {
		return
	}

	s.mu.Lock()
	s.latest = nil
	s.mu.Unlock()

	path := filepath.Join(s.dir, key+".json")
	tmp, err := os.CreateTemp(s.dir, "tmp-*")

	if err != nil {
		return
	}

	defer func() {
		if err := os.Remove(tmp.Name()); err != nil && !os.IsNotExist(err) {
			return
		}
	}()

	if _, err := tmp.Write(body); err != nil {
		if cerr := tmp.Close(); cerr != nil {
			return
		}
		return
	}

	if err := tmp.Close(); err != nil {
		return
	}

	_ = os.Rename(tmp.Name(), path)
}

func (s *Store) LastFor(name string) (Fingerprint, bool) {
	if !s.mode.Read {
		return Fingerprint{}, false
	}

	rec, ok := s.newest(name)

	return rec.Fingerprint, ok
}

func (s *Store) LastDuration(name string) time.Duration {
	rec, _ := s.newest(name)

	return time.Duration(rec.DurationMS) * time.Millisecond
}

func (s *Store) newest(name string) (record, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.latest == nil {
		s.latest = s.index()
	}

	rec, ok := s.latest[name]

	return rec, ok
}

func (s *Store) index() map[string]record {
	out := map[string]record{}

	entries, err := os.ReadDir(s.dir)

	if err != nil {
		return out
	}

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}

		body, err := os.ReadFile(filepath.Join(s.dir, e.Name()))

		if err != nil {
			continue
		}

		var rec record

		if err := json.Unmarshal(body, &rec); err != nil || rec.Name == "" {
			continue
		}

		if prev, seen := out[rec.Name]; !seen || rec.RecordedAt.After(prev.RecordedAt) {
			out[rec.Name] = rec
		}
	}

	return out
}
