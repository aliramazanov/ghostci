package hook

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", ".")
	runGit(t, dir, "config", "user.email", "t@t.t")
	runGit(t, dir, "config", "user.name", "t")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestInstallAndUninstall(t *testing.T) {
	t.Parallel()
	dir := newRepo(t)

	path, err := Install(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if !Installed(dir) {
		t.Error("hook should report as installed")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("hook is not executable: %v", info.Mode())
	}

	if _, err := Uninstall(dir); err != nil {
		t.Fatal(err)
	}
	if Installed(dir) {
		t.Error("hook should be gone")
	}
}

func TestInstallIsIdempotent(t *testing.T) {
	t.Parallel()
	dir := newRepo(t)
	first, err := Install(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := os.ReadFile(first)
	if _, err := Install(dir, false); err != nil {
		t.Fatalf("reinstalling over our own hook should succeed: %v", err)
	}
	b, _ := os.ReadFile(first)
	if string(a) != string(b) {
		t.Error("reinstall changed the hook body")
	}
}

func TestRefusesForeignHook(t *testing.T) {
	t.Parallel()
	dir := newRepo(t)
	path, err := Path(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	handWritten := "#!/bin/sh\necho mine\n"
	if err := os.WriteFile(path, []byte(handWritten), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := Install(dir, false); !errors.Is(err, ErrForeignHook) {
		t.Fatalf("want ErrForeignHook, got %v", err)
	}
	if body, _ := os.ReadFile(path); string(body) != handWritten {
		t.Error("a foreign hook must not be modified")
	}

	if _, err := Uninstall(dir); !errors.Is(err, ErrForeignHook) {
		t.Errorf("uninstall must not remove a foreign hook, got %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Error("foreign hook was deleted")
	}

	if _, err := Install(dir, true); err != nil {
		t.Fatalf("--force should replace it: %v", err)
	}
	if !Installed(dir) {
		t.Error("force install did not take")
	}
}

func TestUninstallWhenAbsent(t *testing.T) {
	t.Parallel()
	if _, err := Uninstall(newRepo(t)); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("want ErrNotInstalled, got %v", err)
	}
}

func TestWorktreeHooksResolveToCommonDir(t *testing.T) {
	t.Parallel()
	main := newRepo(t)
	if err := os.WriteFile(filepath.Join(main, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, main, "add", "-A")
	runGit(t, main, "commit", "-qm", "init")

	wt := filepath.Join(t.TempDir(), "wt")
	runGit(t, main, "worktree", "add", "-q", "-b", "side", wt)

	if fi, err := os.Stat(filepath.Join(wt, ".git")); err != nil || fi.IsDir() {
		t.Fatalf("expected .git to be a file inside a worktree")
	}

	path, err := Path(wt)
	if err != nil {
		t.Fatalf("hook path resolution failed inside a worktree: %v", err)
	}
	if strings.Contains(path, filepath.Join(".git", "worktrees")) {
		t.Errorf("hooks resolved into the per-worktree dir, git will not run them: %s", path)
	}
	if _, err := Install(wt, false); err != nil {
		t.Fatalf("install inside a worktree: %v", err)
	}

	if !Installed(main) {
		t.Error("hook installed from a worktree is not visible to the main checkout")
	}
}

func TestRespectsCoreHooksPath(t *testing.T) {
	t.Parallel()
	dir := newRepo(t)
	custom := filepath.Join(dir, "myhooks")
	runGit(t, dir, "config", "core.hooksPath", "myhooks")

	path, err := Install(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := realPath(filepath.Dir(path)), realPath(custom); got != want {
		t.Errorf("hook written to %s, want %s", got, want)
	}
}

func realPath(path string) string {
	if p, err := filepath.EvalSymlinks(path); err == nil {
		return p
	}

	parent, base := filepath.Split(path)
	if p, err := filepath.EvalSymlinks(filepath.Clean(parent)); err == nil {
		return filepath.Join(p, base)
	}

	return path
}

func TestScriptDrainsStdinOnEveryExitPath(t *testing.T) {
	t.Parallel()
	body := mustScript(t)
	blocks := strings.Split(body, "exit 0")
	if len(blocks) < 3 {
		t.Fatalf("expected multiple early-exit paths in the hook")
	}
	for i, b := range blocks[:len(blocks)-1] {
		if !strings.Contains(b, "cat >/dev/null") {
			t.Errorf("exit path %d does not drain stdin:\n%s", i, b)
		}
	}
	if !strings.Contains(body, "exec ghostci --hook") {
		t.Error("hook should exec ghostci in the normal path")
	}
	if !strings.Contains(body, marker) {
		t.Error("hook must carry the managed marker")
	}
}

func TestScriptIsValidShell(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "pre-push")
	if err := os.WriteFile(path, []byte(mustScript(t)), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("sh", "-n", path).CombinedOutput(); err != nil {
		t.Fatalf("hook is not valid shell: %v: %s", err, out)
	}
}

func TestReadRefs(t *testing.T) {
	t.Parallel()
	const zero = "0000000000000000000000000000000000000000"
	in := strings.NewReader(
		"refs/heads/main abc123 refs/heads/main def456\n" +
			"refs/heads/new aaa111 refs/heads/new " + zero + "\n" +
			"(delete) " + zero + " refs/heads/old bbb222\n" +
			"garbage line\n")

	refs := ReadRefs(in)
	if len(refs) != 3 {
		t.Fatalf("got %d refs, want 3", len(refs))
	}
	if refs[0].LocalRef != "refs/heads/main" || refs[0].RemoteSHA != "def456" {
		t.Errorf("bad parse: %+v", refs[0])
	}
	if !refs[1].NewBranch() {
		t.Error("zero remote sha means a new branch")
	}
	if refs[0].NewBranch() {
		t.Error("existing remote sha is not a new branch")
	}
	if !refs[2].Deleted() {
		t.Error("zero local sha means a deletion")
	}
	if refs[0].Deleted() {
		t.Error("false positive on deletion")
	}
}

func TestOutsideRepo(t *testing.T) {
	t.Parallel()
	if _, err := Path(t.TempDir()); err == nil {
		t.Error("want an error outside a git repo")
	}
}

func TestRefusesHooksPathOutsideRepo(t *testing.T) {
	t.Parallel()

	dir := newRepo(t)
	shared := t.TempDir()
	runGit(t, dir, "config", "core.hooksPath", shared)

	_, err := Install(dir, false)

	var outside *ErrSharedHooksPath
	if !errors.As(err, &outside) {
		t.Fatalf("want ErrSharedHooksPath, got %v", err)
	}
	if entries, _ := os.ReadDir(shared); len(entries) != 0 {
		t.Error("nothing may be written to a shared hooks directory without --force")
	}

	if _, err := Install(dir, true); err != nil {
		t.Fatalf("--force should allow it: %v", err)
	}
	if entries, _ := os.ReadDir(shared); len(entries) != 1 {
		t.Error("--force should install")
	}
}

func TestAllowsHooksPathInsideRepo(t *testing.T) {
	t.Parallel()

	dir := newRepo(t)
	runGit(t, dir, "config", "core.hooksPath", ".githooks")

	if _, err := Install(dir, false); err != nil {
		t.Fatalf("a repo-local hooks dir should install without --force: %v", err)
	}
}

func mustScript(t *testing.T) string {
	t.Helper()

	s, err := hookScript()
	if err != nil {
		t.Fatal(err)
	}

	return s
}

func TestHookFindsBinaryOutsidePath(t *testing.T) {
	t.Parallel()

	script := mustScript(t)

	if !strings.Contains(script, "GHOSTCI_BIN") {
		t.Error("hook should honour an explicit binary override")
	}
	if !strings.Contains(script, "NO CHECKS RAN") {
		t.Error("a hook that cannot find ghostci must say so loudly")
	}
	if !strings.Contains(script, `--hook "$@"`) {
		t.Error("git's hook arguments should be forwarded")
	}

	self, _ := os.Executable()
	if self != "" && !strings.Contains(script, filepath.Base(self)) {
		t.Errorf("hook should record the installing binary path, got:\n%s", script)
	}
}

func TestHookVerboseSwitch(t *testing.T) {
	t.Parallel()

	if !strings.Contains(mustScript(t), "GHOSTCI_VERBOSE") {
		t.Error("hook should support tracing itself")
	}
}

func TestRealPathComparesTwoSpellingsOfOneDirectory(t *testing.T) {
	t.Parallel()

	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")

	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if realPath(link) != realPath(real) {
		t.Errorf("realPath(%q) = %q, want the same directory as %q", link, realPath(link), realPath(real))
	}

	viaLink := realPath(filepath.Join(link, "hooks"))
	direct := realPath(filepath.Join(real, "hooks"))

	if viaLink != direct {
		t.Errorf("realPath = %q via the link and %q directly", viaLink, direct)
	}
}

func TestInstallFromASubdirectoryUsesTheRepositorysHooks(t *testing.T) {
	t.Parallel()
	dir := newRepo(t)
	sub := filepath.Join(dir, "sub", "deeper")

	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	path, err := Install(sub, false)
	if err != nil {
		t.Fatalf("installing from a subdirectory failed: %v", err)
	}

	want := filepath.Join(realPath(dir), ".git", "hooks", Name)
	if realPath(path) != want {
		t.Errorf("installed at %s, want %s", path, want)
	}

	if !Installed(dir) {
		t.Error("the repository's own hook is missing")
	}
}
