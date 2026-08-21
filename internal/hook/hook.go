package hook

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aliramazanov/ghostci/internal/git"
	"github.com/aliramazanov/ghostci/internal/templates"
)

const (
	Name   = "pre-push"
	marker = "# ghostci:managed"

	backupSuffix = ".old"
)

var (
	ErrForeignHook  = errors.New("hook: a pre-push hook already exists that ghostci did not write")
	ErrNotInstalled = errors.New("hook: no ghostci hook installed")

	ErrEmptyHooksPath = errors.New(
		"hook: core.hooksPath is set to an empty value; unset it or point it at a directory")
)

type ErrSharedHooksPath struct {
	Dir  string
	Root string
}

func (e *ErrSharedHooksPath) Error() string {
	return "hook: core.hooksPath is " + e.Dir + ", outside " + e.Root
}

func Dir(repoDir string) (string, error) {

	if custom, err := git.HooksPathConfig(repoDir); err == nil {
		if custom == "" {
			return "", ErrEmptyHooksPath
		}
		if filepath.IsAbs(custom) {
			return custom, nil
		}

		root, err := git.Root(repoDir)
		if err != nil {
			return "", err
		}
		return filepath.Join(root, custom), nil
	}

	path, err := git.HooksDir(repoDir)
	if err != nil {
		return "", fmt.Errorf("hook: not a git repository: %w", err)
	}
	if filepath.IsAbs(path) {
		return path, nil
	}
	root, err := git.Root(repoDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, path), nil
}

func Path(repoDir string) (string, error) {
	dir, err := Dir(repoDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, Name), nil
}

func Install(repoDir string, force bool) (path string, err error) {
	path, err = Path(repoDir)
	if err != nil {
		return "", err
	}

	if err := checkShared(repoDir, path); err != nil && !force {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("hook: creating %s: %w", filepath.Dir(path), err)
	}

	if existing, err := os.ReadFile(path); err == nil {
		if !isManaged(existing) {
			if !force {
				return "", ErrForeignHook
			}
			if err := backup(path); err != nil {
				return "", err
			}
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}

	script, err := hookScript()
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		return "", fmt.Errorf("hook: writing %s: %w", path, err)
	}
	return path, nil
}

func Uninstall(repoDir string) (path string, err error) {
	path, err = Path(repoDir)
	if err != nil {
		return "", err
	}
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", ErrNotInstalled
	}
	if err != nil {
		return "", err
	}
	if !isManaged(body) {
		return "", ErrForeignHook
	}
	if err := os.Remove(path); err != nil {
		return "", fmt.Errorf("hook: removing %s: %w", path, err)
	}

	if err := os.Rename(path+backupSuffix, path); err != nil && !os.IsNotExist(err) {
		return path, fmt.Errorf("hook: restoring %s: %w", path+backupSuffix, err)
	}

	return path, nil
}

type ErrBackupExists struct{ Path string }

func (e *ErrBackupExists) Error() string {
	return "hook: " + e.Path + " already holds a saved hook; move or delete it first"
}

func backup(path string) error {
	if _, err := os.Stat(path + backupSuffix); err == nil {
		return &ErrBackupExists{Path: path + backupSuffix}
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.Rename(path, path+backupSuffix); err != nil {
		return fmt.Errorf("hook: saving the existing hook: %w", err)
	}

	return nil
}

func Installed(repoDir string) bool {
	path, err := Path(repoDir)
	if err != nil {
		return false
	}
	body, err := os.ReadFile(path)
	return err == nil && isManaged(body)
}

func isManaged(body []byte) bool {
	return strings.Contains(string(body), marker)
}

type Ref struct {
	LocalRef  string
	LocalSHA  string
	RemoteRef string
	RemoteSHA string
}

func (r Ref) Deleted() bool {
	return r.LocalSHA == "" || strings.Trim(r.LocalSHA, "0") == ""
}

func (r Ref) NewBranch() bool {
	return strings.Trim(r.RemoteSHA, "0") == ""
}

func ReadRefs(r io.Reader) []Ref {
	var refs []Ref
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		refs = append(refs, Ref{
			LocalRef:  fields[0],
			LocalSHA:  fields[1],
			RemoteRef: fields[2],
			RemoteSHA: fields[3],
		})
	}
	return refs
}

func checkShared(repoDir, hookPath string) error {

	dir, err := filepath.Abs(filepath.Dir(hookPath))
	if err != nil {
		return nil
	}

	root, err := git.Root(repoDir)
	if err != nil {
		return nil
	}

	owned := []string{root}

	if common, err := git.CommonDir(repoDir); err == nil && common != "" {
		if !filepath.IsAbs(common) {
			common = filepath.Join(repoDir, common)
		}

		if abs, err := filepath.Abs(common); err == nil {
			owned = append(owned, abs)
		}
	}

	for _, base := range owned {
		if within(base, dir) {
			return nil
		}
	}

	return &ErrSharedHooksPath{Dir: dir, Root: root}
}

func within(base, target string) bool {
	absBase, err := filepath.Abs(base)
	if err != nil {
		return false
	}

	rel, err := filepath.Rel(absBase, target)
	if err != nil {
		return false
	}

	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func hookScript() (string, error) {
	self, err := os.Executable()
	if err != nil {
		self = ""
	}

	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}

	script, err := templates.PrePush(templates.PrePushParams{Binary: self})
	if err != nil {
		return "", fmt.Errorf("hook: rendering script: %w", err)
	}

	return script, nil
}
