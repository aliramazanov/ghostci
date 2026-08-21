package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/aliramazanov/ghostci/internal/git"
)

type Tree struct {
	root  string
	blobs map[string]string
	dirty map[string]bool
}

func ScanTree(sess *git.Session) (*Tree, error) {
	root, err := sess.Root()
	if err != nil {
		return nil, fmt.Errorf("cache: locating repository root: %w", err)
	}

	t := &Tree{
		root:  root,
		blobs: map[string]string{},
		dirty: map[string]bool{},
	}

	blobs, err := sess.Blobs()
	if err != nil {
		return nil, fmt.Errorf("cache: reading git index: %w", err)
	}

	t.blobs = blobs

	dirty, err := sess.Dirty()

	if err != nil {
		return nil, fmt.Errorf("cache: reading working tree status: %w", err)
	}

	for _, p := range dirty {
		t.dirty[p] = true
	}

	return t, nil
}

const Dir = ".ghostci"

func ours(path string) bool {
	return path == Dir || strings.HasPrefix(path, Dir+"/")
}

func (t *Tree) Hashes(match func(file string) bool) map[string]string {
	out := map[string]string{}

	for p := range t.blobs {
		if t.dirty[p] || ours(p) {
			continue
		}
		if match(clean(p)) {
			out[p] = "b:" + t.blobs[p]
		}
	}

	for p := range t.dirty {
		if ours(p) || !match(clean(p)) {
			continue
		}
		h, err := hashFile(filepath.Join(t.root, p))
		if err != nil {
			out[p] = "gone"
			continue
		}
		out[p] = "c:" + h
	}

	return out
}

func clean(file string) string { return path.Clean(file) }

func hashFile(path string) (string, error) {
	f, err := os.Open(path)

	if err != nil {
		return "", err
	}

	h := sha256.New()

	if _, err := io.Copy(h, f); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("cache: hashing %s: %w", path, err)
	}

	if err := f.Close(); err != nil {
		return "", fmt.Errorf("cache: closing %s: %w", path, err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
