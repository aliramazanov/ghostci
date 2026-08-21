package main

import (
	"slices"
	"testing"

	"github.com/aliramazanov/ghostci/internal/hook"
)

const (
	headSHAFixture = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	otherSHA       = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	remoteTip      = "cccccccccccccccccccccccccccccccccccccccc"
	zeroSHA        = "0000000000000000000000000000000000000000"
)

type pushCase struct {
	name      string
	refs      []hook.Ref
	head      string
	wantBase  string
	wantOK    bool
	wantUnchk []string

	deref map[string]string
}

func (c pushCase) commitOf() func(string) string {
	return func(sha string) string {
		if commit, ok := c.deref[sha]; ok {
			return commit
		}

		return sha
	}
}

func TestPushBase(t *testing.T) {
	t.Parallel()
	cases := []pushCase{
		{
			name:   "nothing live leaves the base to be inferred",
			refs:   []hook.Ref{{LocalRef: "refs/heads/gone", LocalSHA: zeroSHA, RemoteSHA: remoteTip}},
			head:   headSHAFixture,
			wantOK: true,
		},
		{
			name:     "the checked-out ref supplies its remote tip",
			refs:     []hook.Ref{{LocalRef: "refs/heads/main", LocalSHA: headSHAFixture, RemoteSHA: remoteTip}},
			head:     headSHAFixture,
			wantBase: remoteTip,
			wantOK:   true,
		},
		{
			name:   "a branch the remote has never seen is inferred",
			refs:   []hook.Ref{{LocalRef: "refs/heads/new", LocalSHA: headSHAFixture, RemoteSHA: zeroSHA}},
			head:   headSHAFixture,
			wantOK: true,
		},
		{

			name:   "a ref that is not checked out cannot be spoken to",
			refs:   []hook.Ref{{LocalRef: "refs/heads/other", LocalSHA: otherSHA, RemoteSHA: remoteTip}},
			head:   headSHAFixture,
			wantOK: false,
		},
		{

			name: "a multi-ref push still checks the ref on disk",
			refs: []hook.Ref{
				{LocalRef: "refs/heads/main", LocalSHA: headSHAFixture, RemoteSHA: remoteTip},
				{LocalRef: "refs/heads/other", LocalSHA: otherSHA, RemoteSHA: remoteTip},
			},
			head:      headSHAFixture,
			wantBase:  remoteTip,
			wantOK:    true,
			wantUnchk: []string{"refs/heads/other"},
		},
		{
			name: "refs at the same commit with different tips are inferred",
			refs: []hook.Ref{
				{LocalRef: "refs/heads/main", LocalSHA: headSHAFixture, RemoteSHA: remoteTip},
				{LocalRef: "refs/tags/v1", LocalSHA: headSHAFixture, RemoteSHA: zeroSHA},
			},
			head:   headSHAFixture,
			wantOK: true,
		},
		{
			name: "no live ref matches head, so nothing is verified",
			refs: []hook.Ref{
				{LocalRef: "refs/heads/a", LocalSHA: otherSHA, RemoteSHA: remoteTip},
				{LocalRef: "refs/heads/b", LocalSHA: otherSHA, RemoteSHA: remoteTip},
			},
			head:   headSHAFixture,
			wantOK: false,
		},
		{
			name:   "an unknown head cannot anchor anything",
			refs:   []hook.Ref{{LocalRef: "refs/heads/main", LocalSHA: headSHAFixture, RemoteSHA: remoteTip}},
			head:   "",
			wantOK: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			base, ok, unchecked := pushBase(c.refs, c.head, c.commitOf())

			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}

			if base != c.wantBase {
				t.Errorf("base = %q, want %q", base, c.wantBase)
			}

			if !slices.Equal(unchecked, c.wantUnchk) {
				t.Errorf("unchecked = %v, want %v", unchecked, c.wantUnchk)
			}
		})
	}
}

func TestAnnotatedTagOnHeadIsCheckable(t *testing.T) {
	t.Parallel()

	const tagObject = "dddddddddddddddddddddddddddddddddddddddd"

	deref := func(sha string) string {
		if sha == tagObject {
			return headSHAFixture
		}

		return sha
	}

	refs := []hook.Ref{
		{LocalRef: "refs/tags/v1.0.0", LocalSHA: tagObject, RemoteSHA: zeroSHA},
	}

	base, ok, unchecked := pushBase(refs, headSHAFixture, deref)
	if !ok {
		t.Fatal("a tag pointing at HEAD was reported as unverifiable")
	}
	if base != "" {
		t.Errorf("base = %q, want the base inferred for a ref the remote has never seen", base)
	}
	if len(unchecked) != 0 {
		t.Errorf("unchecked = %v, want none", unchecked)
	}

	other := []hook.Ref{
		{LocalRef: "refs/tags/old", LocalSHA: otherSHA, RemoteSHA: zeroSHA},
	}
	if _, ok, _ := pushBase(other, headSHAFixture, deref); ok {
		t.Error("a tag pointing at another commit was treated as checkable")
	}
}
