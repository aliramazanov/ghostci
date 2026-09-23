package cache

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aliramazanov/ghostci/internal/config"
	"github.com/aliramazanov/ghostci/internal/git"
)

func fp(inputs map[string]string) Fingerprint {
	return New(config.Check{Command: "go test ./...", Inputs: []string{"**/*.go"}}, inputs, nil)
}

func TestKeyIsDeterministic(t *testing.T) {
	t.Parallel()
	inputs := map[string]string{"a.go": "1", "b.go": "2", "c.go": "3", "d.go": "4"}

	first, err := fp(inputs).Key()
	if err != nil {
		t.Fatal(err)
	}

	for range 20 {
		again, err := fp(map[string]string{"d.go": "4", "c.go": "3", "b.go": "2", "a.go": "1"}).Key()

		if err != nil {
			t.Fatal(err)
		}

		if again != first {
			t.Fatalf("key is not stable: %s != %s", again, first)
		}
	}
}

func TestKeyChangesWithEachField(t *testing.T) {
	t.Parallel()

	base := New(
		config.Check{
			Command: "go test",
			Dir:     "x",
			Shell:   "bash",
			Env:     map[string]string{"A": "1"},
		},
		map[string]string{"a.go": "h1"},
		map[string]string{"go": "1.26"},
	)

	baseKey, _ := base.Key()

	variants := map[string]Fingerprint{
		"command": func() Fingerprint { f := base; f.Command = "go vet"; return f }(),
		"dir":     func() Fingerprint { f := base; f.Dir = "y"; return f }(),
		"shell":   func() Fingerprint { f := base; f.Shell = "sh"; return f }(),

		"env": func() Fingerprint {
			f := base
			f.Env = map[string]string{"A": "2"}
			return f
		}(),

		"input": func() Fingerprint {
			f := base
			f.Inputs = map[string]string{"a.go": "h2"}
			return f
		}(),

		"new input": func() Fingerprint {
			f := base
			f.Inputs = map[string]string{"a.go": "h1", "b.go": "h3"}
			return f
		}(),

		"toolchain": func() Fingerprint {
			f := base
			f.Toolchains = map[string]string{"go": "1.25"}
			return f
		}(),

		"version": func() Fingerprint { f := base; f.Version = "999"; return f }(),
	}

	for name, v := range variants {

		t.Run(name, func(t *testing.T) {
			t.Parallel()
			k, _ := v.Key()

			if k == baseKey {
				t.Errorf("changing %s did not change the key", name)
			}
		})
	}
}

func TestDiffExplainsTheMiss(t *testing.T) {
	t.Parallel()
	before := fp(map[string]string{"a.go": "1", "gone.go": "9"})
	now := fp(map[string]string{"a.go": "2", "new.go": "3"})

	joined := strings.Join(now.Diff(before), "; ")

	for _, want := range []string{"changed: a.go", "added: new.go", "removed: gone.go"} {
		if !strings.Contains(joined, want) {
			t.Errorf("diff missing %q: %s", want, joined)
		}
	}
}

func TestStoreRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := Open(dir, ReadWrite())
	f := fp(map[string]string{"a.go": "1"})

	if hit, _ := s.Lookup("test", f); hit {
		t.Fatal("empty store reported a hit")
	}

	s.Record("test", f, 250*time.Millisecond)

	if hit, _ := s.Lookup("test", f); !hit {
		t.Fatal("recorded fingerprint did not hit")
	}

	changed := fp(map[string]string{"a.go": "2"})

	if hit, _ := s.Lookup("test", changed); hit {
		t.Fatal("a changed input must miss")
	}
}

func TestStoreDisabled(t *testing.T) {
	t.Parallel()
	s := Open(t.TempDir(), Off())
	f := fp(map[string]string{"a.go": "1"})
	s.Record("test", f, time.Second)

	if hit, _ := s.Lookup("test", f); hit {
		t.Error("a disabled store must never hit")
	}
}

func TestLastForDiffing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := Open(dir, ReadWrite())
	s.Record("test", fp(map[string]string{"a.go": "1"}), time.Second)

	previous, ok := s.LastFor("test")

	if !ok {
		t.Fatal("no previous fingerprint found")
	}

	if previous.Inputs["a.go"] != "1" {
		t.Errorf("wrong fingerprint: %+v", previous.Inputs)
	}

	if _, ok := s.LastFor("nonexistent"); ok {
		t.Error("found a fingerprint for a check that never ran")
	}
}

func TestCorruptRecordIsAMiss(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := Open(dir, ReadWrite())
	f := fp(map[string]string{"a.go": "1"})
	s.Record("test", f, time.Second)

	key, _ := f.Key()
	path := filepath.Join(s.checkDir("test"), key+".json")

	if err := os.WriteFile(path, []byte("{ truncated"), 0o644); err != nil {
		t.Fatal(err)
	}

	if hit, _ := s.Lookup("test", f); hit {
		t.Error("a corrupt record must not count as a hit")
	}
}

func recordAt(t *testing.T, s *Store, name string, f Fingerprint, at time.Time) string {
	t.Helper()

	s.Record(name, f, time.Second)

	key, _ := f.Key()
	path := filepath.Join(s.checkDir(name), key+".json")

	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}

	return path
}

func version(n int) Fingerprint {
	return fp(map[string]string{"a.go": strconv.Itoa(n)})
}

func TestHistoryIsBoundedPerCheck(t *testing.T) {
	t.Parallel()
	s := Open(t.TempDir(), ReadWrite())
	start := time.Now().Add(-time.Hour)
	total := keepPerCheck + 5

	other := recordAt(t, s, "other", version(0), start)

	for i := range total {
		recordAt(t, s, "test", version(i), start.Add(time.Duration(i+1)*time.Second))
	}

	if got := len(history(s.checkDir("test"))); got != keepPerCheck {
		t.Errorf("kept %d records, want %d", got, keepPerCheck)
	}

	if hit, _ := s.Lookup("test", version(total-keepPerCheck-1)); hit {
		t.Error("the oldest record outlived the bound")
	}

	if hit, _ := s.Lookup("test", version(total-keepPerCheck)); !hit {
		t.Error("a record inside the bound was dropped")
	}

	if _, err := os.Stat(other); err != nil {
		t.Errorf("recording one check dropped another's history: %v", err)
	}
}

func TestAStateThatKeepsHittingIsKept(t *testing.T) {
	t.Parallel()
	s := Open(t.TempDir(), ReadWrite())
	start := time.Now().Add(-time.Hour)

	recordAt(t, s, "test", version(0), start)

	for i := 1; i < keepPerCheck; i++ {
		recordAt(t, s, "test", version(i), start.Add(time.Duration(i)*time.Second))
	}

	if hit, _ := s.Lookup("test", version(0)); !hit {
		t.Fatal("the first state should still be cached")
	}

	s.Record("test", version(keepPerCheck), time.Second)

	if hit, _ := s.Lookup("test", version(0)); !hit {
		t.Error("a state that just hit was evicted ahead of ones nobody used")
	}

	if hit, _ := s.Lookup("test", version(1)); hit {
		t.Error("the least recently used state should have made room")
	}
}

func TestLastForIsTheNewestRecord(t *testing.T) {
	t.Parallel()
	s := Open(t.TempDir(), ReadWrite())
	start := time.Now().Add(-time.Hour)

	recordAt(t, s, "test", version(2), start.Add(2*time.Second))
	recordAt(t, s, "test", version(1), start.Add(time.Second))

	previous, ok := s.LastFor("test")

	if !ok || previous.Inputs["a.go"] != "2" {
		t.Errorf("LastFor = %+v, %v; want the most recent record", previous.Inputs, ok)
	}
}

func TestFlatRecordsFromOlderVersionsAreRemoved(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	flat := filepath.Join(root, ".ghostci", "cache")

	if err := os.MkdirAll(flat, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"0123abcd.json", "tmp-42"} {
		if err := os.WriteFile(filepath.Join(flat, name), []byte(`{"name":"test"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	Open(root, ReadWrite()).Record("test", version(1), time.Second)

	entries, err := os.ReadDir(flat)
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			t.Errorf("%s survived; the cache root should hold only per-check directories", e.Name())
		}
	}
}

func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main", "."},
		{"config", "user.email", "t@t.t"},
		{"config", "user.name", "t"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}

	return dir
}

func commit(t *testing.T, dir string) {
	t.Helper()

	for _, args := range [][]string{{"add", "-A"}, {"commit", "-qm", "x"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
}

func onlyGo(file string) bool { return strings.HasSuffix(file, ".go") }

func TestTreeHashesCleanAndDirtyFiles(t *testing.T) {
	t.Parallel()
	dir := gitRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	commit(t, dir)

	tree, err := ScanTree(git.NewSession(dir))

	if err != nil {
		t.Fatal(err)
	}

	clean := tree.Hashes(onlyGo)

	if got := clean["a.go"]; !strings.HasPrefix(got, "b:") {
		t.Errorf("a clean tracked file should use the index blob sha, got %q", got)
	}

	if err := os.WriteFile(
		filepath.Join(dir, "a.go"),
		[]byte("package a\n\nfunc X() {}\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	tree2, err := ScanTree(git.NewSession(dir))

	if err != nil {
		t.Fatal(err)
	}

	dirty := tree2.Hashes(onlyGo)

	if got, want := dirty["a.go"], "b:"+hashObject(t, dir, "a.go"); got != want {
		t.Errorf("a dirty file should hash to the blob git would store, got %q want %q", got, want)
	}

	if dirty["a.go"] == clean["a.go"] {
		t.Error("editing a file must change its identity")
	}
}

func hashObject(t *testing.T, dir, file string) string {
	t.Helper()

	cmd := exec.Command("git", "hash-object", file)
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}

	return strings.TrimSpace(string(out))
}

func TestCommittingAFileKeepsItsIdentity(t *testing.T) {
	t.Parallel()
	dir := gitRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	commit(t, dir)

	for _, body := range []string{"package a\n\nfunc X() {}\n", "package a\n\nfunc Y() {}\n"} {
		if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(dir, "new.go"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}

		before, err := ScanTree(git.NewSession(dir))
		if err != nil {
			t.Fatal(err)
		}

		dirty := before.Hashes(onlyGo)

		commit(t, dir)

		after, err := ScanTree(git.NewSession(dir))
		if err != nil {
			t.Fatal(err)
		}

		committed := after.Hashes(onlyGo)

		for _, file := range []string{"a.go", "new.go"} {
			if dirty[file] != committed[file] {
				t.Errorf("committing %s unchanged changed its identity: %q before, %q after",
					file, dirty[file], committed[file])
			}
		}
	}
}

func TestTreeIncludesUntracked(t *testing.T) {
	t.Parallel()
	dir := gitRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	commit(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "new.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tree, err := ScanTree(git.NewSession(dir))

	if err != nil {
		t.Fatal(err)
	}

	if _, ok := tree.Hashes(onlyGo)["new.go"]; !ok {
		t.Error("an untracked file must be part of the fingerprint")
	}
}

func TestDeletedFileChangesFingerprint(t *testing.T) {
	t.Parallel()
	dir := gitRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	commit(t, dir)

	tree, _ := ScanTree(git.NewSession(dir))
	before := tree.Hashes(onlyGo)

	if err := os.Remove(filepath.Join(dir, "a.go")); err != nil {
		t.Fatal(err)
	}

	tree2, _ := ScanTree(git.NewSession(dir))
	after := tree2.Hashes(onlyGo)

	if before["a.go"] == after["a.go"] {
		t.Error("deleting a file must change the fingerprint")
	}
}

func TestExcludedFilesLeaveTheFingerprint(t *testing.T) {
	t.Parallel()

	dir := gitRepo(t)

	for _, f := range []string{"a.go", "vendor.go"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("package a\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	commit(t, dir)

	tree, err := ScanTree(git.NewSession(dir))

	if err != nil {
		t.Fatal(err)
	}

	all := tree.Hashes(func(f string) bool { return strings.HasSuffix(f, ".go") })

	some := tree.Hashes(func(f string) bool {
		return strings.HasSuffix(f, ".go") && f != "vendor.go"
	})

	if len(all) != 2 || len(some) != 1 {
		t.Fatalf("all=%d some=%d", len(all), len(some))
	}

	if _, ok := some["vendor.go"]; ok {
		t.Error("an excluded file must not enter the fingerprint")
	}
}

func TestDisabledStoreWritesNothingToDisk(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	f := fp(map[string]string{"a.go": "1"})

	Open(root, Off()).Record("test", f, time.Second)

	if hit, _ := Open(root, ReadWrite()).Lookup("test", f); hit {
		t.Error("a disabled store wrote a record that a reading store then served")
	}
}

func TestBypassSkipsLookupsButStillRecords(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	f := fp(map[string]string{"a.go": "1"})

	bypass := Open(root, Bypass())
	bypass.Record("test", f, time.Second)

	if hit, _ := bypass.Lookup("test", f); hit {
		t.Error("bypass served a cache hit")
	}
	if hit, _ := Open(root, ReadWrite()).Lookup("test", f); !hit {
		t.Error("bypass did not record, so the next run pays for it again")
	}
}

func TestEveryFingerprintFieldChangesTheKey(t *testing.T) {
	t.Parallel()

	base := New(config.Check{
		Name: "c", Command: "go test ./...", Dir: "sub", Shell: "bash -e",
		Env: map[string]string{"MODE": "one"},
	}, map[string]string{"a.go": "1"}, map[string]string{"go": "1.26"})

	baseKey, err := base.Key()
	if err != nil {
		t.Fatal(err)
	}

	variants := map[string]Fingerprint{
		"command": New(config.Check{Name: "c", Command: "go test -race ./...", Dir: "sub", Shell: "bash -e",
			Env: map[string]string{"MODE": "one"}}, map[string]string{"a.go": "1"}, map[string]string{"go": "1.26"}),
		"dir": New(config.Check{Name: "c", Command: "go test ./...", Dir: "other", Shell: "bash -e",
			Env: map[string]string{"MODE": "one"}}, map[string]string{"a.go": "1"}, map[string]string{"go": "1.26"}),
		"shell": New(config.Check{Name: "c", Command: "go test ./...", Dir: "sub", Shell: "sh -e",
			Env: map[string]string{"MODE": "one"}}, map[string]string{"a.go": "1"}, map[string]string{"go": "1.26"}),
		"env": New(config.Check{Name: "c", Command: "go test ./...", Dir: "sub", Shell: "bash -e",
			Env: map[string]string{"MODE": "two"}}, map[string]string{"a.go": "1"}, map[string]string{"go": "1.26"}),
		"inputs": New(config.Check{Name: "c", Command: "go test ./...", Dir: "sub", Shell: "bash -e",
			Env: map[string]string{"MODE": "one"}}, map[string]string{"a.go": "2"}, map[string]string{"go": "1.26"}),
		"toolchain": New(config.Check{Name: "c", Command: "go test ./...", Dir: "sub", Shell: "bash -e",
			Env: map[string]string{"MODE": "one"}}, map[string]string{"a.go": "1"}, map[string]string{"go": "1.27"}),
		"shell environment": func() Fingerprint {
			f := base
			f.Ambient = map[string]string{"GOFLAGS": "digest"}
			return f
		}(),
	}

	for field, v := range variants {
		key, err := v.Key()
		if err != nil {
			t.Fatal(err)
		}
		if key == baseKey {
			t.Errorf("changing %s did not change the fingerprint, so a stale pass would be served", field)
		}
	}
}

func TestAnUnsetEnvironmentKeepsExistingKeys(t *testing.T) {
	t.Parallel()

	body, err := json.Marshal(fp(map[string]string{"a.go": "1"}))
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(body), "ambient") {
		t.Errorf("an empty environment reached the key, so every recorded pass would miss once: %s", body)
	}
}

func TestADeletedInputStillChangesTheFingerprint(t *testing.T) {
	t.Parallel()
	dir := gitRepo(t)

	for _, name := range []string{"a.go", "b.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("package a\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	commit(t, dir)

	before, err := ScanTree(git.NewSession(dir))
	if err != nil {
		t.Fatal(err)
	}
	full := before.Hashes(onlyGo)

	if err := os.Remove(filepath.Join(dir, "b.go")); err != nil {
		t.Fatal(err)
	}

	after, err := ScanTree(git.NewSession(dir))
	if err != nil {
		t.Fatal(err)
	}
	reduced := after.Hashes(onlyGo)

	if reduced["b.go"] == full["b.go"] {
		t.Fatalf("a deleted file kept its hash: %q", reduced["b.go"])
	}

	chk := config.Check{Name: "c", Command: "go test ./...", Inputs: []string{"**/*.go"}}
	keyBefore, err := New(chk, full, nil).Key()
	if err != nil {
		t.Fatal(err)
	}
	keyAfter, err := New(chk, reduced, nil).Key()
	if err != nil {
		t.Fatal(err)
	}

	if keyBefore == keyAfter {
		t.Error("deleting a watched file did not invalidate the cache")
	}
}

func TestTheToolsOwnStateIsNotProjectContent(t *testing.T) {
	t.Parallel()
	dir := gitRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".ghostci", "cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, ".ghostci", "cache", "entry.json"), []byte(`{"name":"c"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	commit(t, dir)

	tree, err := ScanTree(git.NewSession(dir))
	if err != nil {
		t.Fatal(err)
	}

	everything := tree.Hashes(func(string) bool { return true })

	for path := range everything {
		if strings.HasPrefix(path, ".ghostci") {
			t.Errorf("the tool's own state reached the tree: %s", path)
		}
	}
	if _, ok := everything["a.go"]; !ok {
		t.Error("real project content went missing")
	}
}

func TestTheCacheHidesItselfWithoutTouchingTheRepository(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root

		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init", "-q", "-b", "main")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")

	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	run("add", "-A")
	run("commit", "-qm", "init")

	store := Open(root, Mode{Read: true, Write: true})
	store.Record("c", New(config.Check{Name: "c", Command: "true"}, map[string]string{"a.txt": "1"}, nil), 0)

	status := exec.Command("git", "status", "--porcelain")
	status.Dir = root
	out, err := status.Output()

	if err != nil {
		t.Fatal(err)
	}

	if len(bytes.TrimSpace(out)) != 0 {
		t.Errorf("the cache is visible to git:\n%s", out)
	}

	if _, err := os.Stat(filepath.Join(root, ".gitignore")); err == nil {
		t.Error("a .gitignore was created in the repository root")
	}
}

func TestHashingDistinguishesContent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")

	if err := os.WriteFile(a, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(b, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}

	ha, err := blobID(a, sha1.New)
	if err != nil {
		t.Fatal(err)
	}

	hb, err := blobID(b, sha1.New)
	if err != nil {
		t.Fatal(err)
	}

	if ha == "" {
		t.Fatal("hashing a readable file produced nothing")
	}

	if ha == hb {
		t.Fatalf("two files with different contents hash the same: %q", ha)
	}

	if err := os.WriteFile(b, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}

	again, err := blobID(b, sha1.New)
	if err != nil {
		t.Fatal(err)
	}

	if again != ha {
		t.Errorf("identical contents hashed differently: %q and %q", ha, again)
	}

	if _, err := blobID(filepath.Join(dir, "absent"), sha1.New); err == nil {
		t.Error("hashing a missing file reported success")
	}
}

func TestATreeSeesAChangeInContent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}

	run("init", "-q", "-b", "main")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	run("add", "-A")
	run("commit", "-qm", "init")

	all := func(string) bool { return true }

	first, err := ScanTree(git.NewSession(dir))
	if err != nil {
		t.Fatal(err)
	}
	before := first.Hashes(all)

	if err := os.WriteFile(path, []byte("after"), 0o644); err != nil {
		t.Fatal(err)
	}

	second, err := ScanTree(git.NewSession(dir))
	if err != nil {
		t.Fatal(err)
	}
	after := second.Hashes(all)

	if before["a.txt"] == "" || after["a.txt"] == "" {
		t.Fatalf("the file is missing from the tree: before=%q after=%q", before["a.txt"], after["a.txt"])
	}
	if before["a.txt"] == after["a.txt"] {
		t.Errorf("editing the file did not change its hash: %q", after["a.txt"])
	}
}

func TestAFingerprintHoldsOnlyWhatTheCheckWatches(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(dir, filepath.FromSlash(rel))

		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("src/a.go", "package a\n")
	write("docs/guide.md", "hello\n")

	run("init", "-q", "-b", "main")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	run("add", "-A")
	run("commit", "-qm", "init")

	write("src/a.go", "package a // edited\n")
	write("docs/guide.md", "hello again\n")
	write(Dir+"/cache/whatever.json", `{"x":1}`)

	tree, err := ScanTree(git.NewSession(dir))

	if err != nil {
		t.Fatal(err)
	}

	goOnly := tree.Hashes(func(f string) bool { return strings.HasSuffix(f, ".go") })

	for p := range goOnly {
		if strings.HasPrefix(p, Dir) {
			t.Errorf("the fingerprint includes ghostci's own %s, so every run invalidates the last", p)
		}

		if !strings.HasSuffix(p, ".go") {
			t.Errorf("the fingerprint includes %s, which the check does not watch", p)
		}
	}

	if _, ok := goOnly["src/a.go"]; !ok {
		t.Errorf("the file the check does watch is missing: %v", goOnly)
	}

	all := tree.Hashes(func(string) bool { return true })

	for p := range all {
		if strings.HasPrefix(p, Dir) {
			t.Errorf("a check watching everything picked up %s", p)
		}
	}
}

func TestDiffNamesWhatActuallyChanged(t *testing.T) {
	t.Parallel()

	base := New(config.Check{
		Name: "c", Command: "go test ./...", Dir: "sub", Shell: "bash -e",
		Env: map[string]string{"MODE": "fast"},
	}, map[string]string{"a.go": "1"}, map[string]string{"go": "1.26"})

	for _, c := range []struct {
		what string
		make func() Fingerprint
		want string
	}{
		{"command", func() Fingerprint {
			f := base
			f.Command = "go vet ./..."

			return f
		}, "command changed"},
		{"directory", func() Fingerprint {
			f := base
			f.Dir = "other"

			return f
		}, "working directory changed"},
		{"shell", func() Fingerprint {
			f := base
			f.Shell = "sh -e"

			return f
		}, "shell changed"},
		{"an input", func() Fingerprint {
			f := base
			f.Inputs = map[string]string{"a.go": "2"}

			return f
		}, "changed: a.go"},
		{"a toolchain", func() Fingerprint {
			f := base
			f.Toolchains = map[string]string{"go": "1.27"}

			return f
		}, "toolchain changed: go"},
	} {
		got := strings.Join(c.make().Diff(base), "; ")
		if !strings.Contains(got, c.want) {
			t.Errorf("changing the %s reported %q, want it to mention %q", c.what, got, c.want)
		}
	}

	if got := base.Diff(base); len(got) != 0 {
		t.Errorf("an identical fingerprint reported %v", got)
	}
}

func TestASymlinkHashesAsTheLinkGitStores(t *testing.T) {
	t.Parallel()
	dir := gitRepo(t)

	for _, name := range []string{"target-a.txt", "target-b.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("same\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.Symlink("target-a.txt", filepath.Join(dir, "link.txt")); err != nil {
		t.Fatal(err)
	}

	onlyLink := func(f string) bool { return f == "link.txt" }

	first, err := ScanTree(git.NewSession(dir))
	if err != nil {
		t.Fatal(err)
	}

	before := first.Hashes(onlyLink)["link.txt"]

	if err := os.Remove(filepath.Join(dir, "link.txt")); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink("target-b.txt", filepath.Join(dir, "link.txt")); err != nil {
		t.Fatal(err)
	}

	second, err := ScanTree(git.NewSession(dir))
	if err != nil {
		t.Fatal(err)
	}

	if after := second.Hashes(onlyLink)["link.txt"]; after == before {
		t.Errorf("pointing the link somewhere else kept its identity %q, so a stale pass would be served", after)
	}

	commit(t, dir)

	committed, err := ScanTree(git.NewSession(dir))
	if err != nil {
		t.Fatal(err)
	}

	if got, want := committed.Hashes(onlyLink)["link.txt"], second.Hashes(onlyLink)["link.txt"]; got != want {
		t.Errorf("committing the link changed its identity: %q before, %q after", want, got)
	}
}
