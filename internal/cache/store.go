package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const keepPerCheck = 32

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

	legacy sync.Once
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

func (s *Store) checkDir(name string) string {
	sum := sha256.Sum256([]byte(name))

	return filepath.Join(s.dir, hex.EncodeToString(sum[:8]))
}

func (s *Store) Lookup(name string, fp Fingerprint) (hit bool, previous Fingerprint) {
	if !s.mode.Read {
		return false, Fingerprint{}
	}

	key, err := fp.Key()

	if err != nil {
		return false, Fingerprint{}
	}

	path := filepath.Join(s.checkDir(name), key+".json")

	rec, ok := readRecord(path)

	if !ok {
		return false, Fingerprint{}
	}

	if s.mode.Write {
		now := time.Now()
		_ = os.Chtimes(path, now, now)
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

	dir := s.checkDir(name)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	hideFromGit(filepath.Dir(s.dir))
	s.legacy.Do(s.dropFlatRecords)

	body, err := json.MarshalIndent(record{
		Name:        name,
		RecordedAt:  time.Now(),
		DurationMS:  took.Milliseconds(),
		Fingerprint: fp,
	}, "", "  ")

	if err != nil {
		return
	}

	tmp, err := os.CreateTemp(dir, "tmp-*")

	if err != nil {
		return
	}

	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()

		return
	}

	if err := tmp.Close(); err != nil {
		return
	}

	if err := os.Rename(tmp.Name(), filepath.Join(dir, key+".json")); err != nil {
		return
	}

	if records := history(dir); len(records) > keepPerCheck {
		for _, old := range records[keepPerCheck:] {
			_ = os.Remove(old)
		}
	}
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
	for _, path := range history(s.checkDir(name)) {
		if rec, ok := readRecord(path); ok {
			return rec, true
		}
	}

	return record{}, false
}

func history(dir string) []string {
	entries, err := os.ReadDir(dir)

	if err != nil {
		return nil
	}

	type dated struct {
		path string
		at   time.Time
	}

	records := make([]dated, 0, len(entries))

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}

		info, err := e.Info()

		if err != nil {
			continue
		}

		records = append(records, dated{filepath.Join(dir, e.Name()), info.ModTime()})
	}

	slices.SortFunc(records, func(a, b dated) int { return b.at.Compare(a.at) })

	out := make([]string, len(records))
	for i, r := range records {
		out[i] = r.path
	}

	return out
}

func readRecord(path string) (record, bool) {
	body, err := os.ReadFile(path)

	if err != nil {
		return record{}, false
	}

	var rec record

	if err := json.Unmarshal(body, &rec); err != nil {
		return record{}, false
	}

	return rec, true
}

func (s *Store) dropFlatRecords() {
	entries, err := os.ReadDir(s.dir)

	if err != nil {
		return
	}

	for _, e := range entries {
		if !e.IsDir() && (filepath.Ext(e.Name()) == ".json" || strings.HasPrefix(e.Name(), "tmp-")) {
			_ = os.Remove(filepath.Join(s.dir, e.Name()))
		}
	}
}

func hideFromGit(dir string) {
	path := filepath.Join(dir, ".gitignore")

	if _, err := os.Stat(path); err == nil {
		return
	}

	_ = os.WriteFile(path, []byte("*\n"), 0o644)
}
