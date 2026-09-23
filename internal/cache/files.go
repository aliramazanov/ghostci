package cache

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/aliramazanov/ghostci/internal/git"
)

type Tree struct {
	root    string
	blobs   map[string]string
	dirty   map[string]bool
	newHash func() hash.Hash
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
	t.newHash = objectFormat(blobs)

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
		id, err := blobID(filepath.Join(t.root, p), t.newHash)
		if err != nil {
			out[p] = "gone"
			continue
		}
		out[p] = "b:" + id
	}

	return out
}

func (t *Tree) Any(match func(file string) bool) bool {
	for p := range t.blobs {
		if !ours(p) && match(clean(p)) {
			return true
		}
	}

	for p := range t.dirty {
		if !ours(p) && match(clean(p)) {
			return true
		}
	}

	return false
}

func clean(file string) string { return path.Clean(file) }

func objectFormat(blobs map[string]string) func() hash.Hash {
	for _, id := range blobs {
		if len(id) == 2*sha256.Size {
			return sha256.New
		}

		return sha1.New
	}

	return sha1.New
}

func blobID(path string, newHash func() hash.Hash) (string, error) {
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return "", fmt.Errorf("cache: reading link %s: %w", path, err)
		}

		h := newHash()
		fmt.Fprintf(h, "blob %d\x00%s", len(target), target)

		return hex.EncodeToString(h.Sum(nil)), nil
	}

	f, err := os.Open(path)

	if err != nil {
		return "", err
	}

	info, err := f.Stat()

	if err != nil {
		_ = f.Close()
		return "", fmt.Errorf("cache: reading %s: %w", path, err)
	}

	h := newHash()
	fmt.Fprintf(h, "blob %d\x00", info.Size())

	n, err := io.Copy(h, f)

	if err != nil {
		_ = f.Close()
		return "", fmt.Errorf("cache: hashing %s: %w", path, err)
	}

	if err := f.Close(); err != nil {
		return "", fmt.Errorf("cache: closing %s: %w", path, err)
	}

	if n != info.Size() {
		return "", fmt.Errorf("cache: %s changed while it was hashed", path)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
