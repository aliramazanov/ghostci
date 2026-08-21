package hook

import (
	"strings"
	"testing"
)

func FuzzReadRefs(f *testing.F) {
	const sha = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const zero = "0000000000000000000000000000000000000000"

	for _, seed := range []string{
		"refs/heads/main " + sha + " refs/heads/main " + zero + "\n",
		"refs/heads/main " + sha + " refs/heads/main " + sha + "\n",
		"refs/tags/v1 " + sha + " refs/tags/v1 " + zero + "\n",
		"(delete) " + zero + " refs/heads/gone " + sha + "\n",
		"",
		"\n\n\n",
		"too few fields\n",
		"a b c d e f g\n",
		strings.Repeat("refs/heads/x "+sha+" refs/heads/x "+zero+"\n", 50),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, src string) {
		refs := ReadRefs(strings.NewReader(src))

		for _, r := range refs {

			_ = r.Deleted()
			_ = r.NewBranch()

			if r.LocalRef == "" && r.LocalSHA == "" && r.RemoteRef == "" && r.RemoteSHA == "" {
				t.Errorf("kept an entirely empty ref from %q", src)
			}
			if strings.ContainsAny(r.LocalSHA, " \t\n") || strings.ContainsAny(r.RemoteSHA, " \t\n") {
				t.Errorf("a sha carries whitespace: %q from %q", r.LocalSHA, src)
			}
		}
	})
}
