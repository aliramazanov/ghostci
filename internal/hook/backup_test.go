package hook

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func hookPath(t *testing.T, dir string) string {
	t.Helper()

	path, err := Path(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestForceKeepsTheHookItReplaces(t *testing.T) {
	dir := gitRepo(t)
	path := hookPath(t, dir)

	if err := os.WriteFile(path, []byte("#!/bin/sh\necho mine\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := Install(dir, true); err != nil {
		t.Fatal(err)
	}

	saved, err := os.ReadFile(path + backupSuffix)
	if err != nil {
		t.Fatalf("the replaced hook was not kept: %v", err)
	}
	if string(saved) != "#!/bin/sh\necho mine\n" {
		t.Errorf("saved hook = %q", saved)
	}
}

func TestUninstallRestoresTheReplacedHook(t *testing.T) {
	dir := gitRepo(t)
	path := hookPath(t, dir)
	original := "#!/bin/sh\necho mine\n"

	if err := os.WriteFile(path, []byte(original), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(dir, true); err != nil {
		t.Fatal(err)
	}
	if _, err := Uninstall(dir); err != nil {
		t.Fatal(err)
	}

	back, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the original hook was not restored: %v", err)
	}
	if string(back) != original {
		t.Errorf("restored hook = %q, want %q", back, original)
	}
	if _, err := os.Stat(path + backupSuffix); !os.IsNotExist(err) {
		t.Error("the backup is still there after restoring")
	}
}

func TestForceRefusesToOverwriteAnExistingBackup(t *testing.T) {
	dir := gitRepo(t)
	path := hookPath(t, dir)

	if err := os.WriteFile(path, []byte("second\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+backupSuffix, []byte("first\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	var target *ErrBackupExists
	if _, err := Install(dir, true); !errors.As(err, &target) {
		t.Fatalf("Install = %v, want ErrBackupExists", err)
	}

	if body, _ := os.ReadFile(path + backupSuffix); string(body) != "first\n" {
		t.Errorf("the earlier backup was disturbed: %q", body)
	}
	if body, _ := os.ReadFile(path); string(body) != "second\n" {
		t.Errorf("the hook was replaced despite the error: %q", body)
	}
}

func TestReinstallDoesNotBackUpItself(t *testing.T) {
	dir := gitRepo(t)
	path := hookPath(t, dir)

	if _, err := Install(dir, false); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(dir, true); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path + backupSuffix); !os.IsNotExist(err) {
		t.Error("reinstalling created a backup of ghostci's own hook")
	}
}
