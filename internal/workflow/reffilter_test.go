package workflow

import "testing"

func parseOn(t *testing.T, on string) *Workflow {
	t.Helper()
	wf, err := Parse([]byte(on + "\njobs: {}\n"))
	if err != nil {
		t.Fatal(err)
	}

	return wf
}

func TestPushMatchesRef(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		on   string
		ref  string
		want bool
	}{
		{"no filters", "on: [push]", "refs/heads/main", true},
		{"tags only excludes branches", "on:\n  push:\n    tags: ['v*']", "refs/heads/main", false},
		{"tags only admits tags", "on:\n  push:\n    tags: ['v*']", "refs/tags/v1.2.3", true},
		{"branch listed", "on:\n  push:\n    branches: [main]", "refs/heads/main", true},
		{"branch not listed", "on:\n  push:\n    branches: [release]", "refs/heads/main", false},
		{"branch glob", "on:\n  push:\n    branches: ['releases/**']", "refs/heads/releases/v2/rc", true},
		{"full ref pattern", "on:\n  push:\n    branches: ['refs/heads/main']", "refs/heads/main", true},
		{"ignore matches", "on:\n  push:\n    branches-ignore: [main]", "refs/heads/main", false},
		{"ignore misses", "on:\n  push:\n    branches-ignore: [docs]", "refs/heads/main", true},
		{"branches only excludes tags", "on:\n  push:\n    branches: [main]", "refs/tags/v1", false},
		{"negation leaves other branches out", "on:\n  push:\n    branches: ['releases/**', '!releases/**-rc']", "refs/heads/main", false},
		{"negation removes what it names", "on:\n  push:\n    branches: ['releases/**', '!releases/**-rc']", "refs/heads/releases/v2-rc", false},
		{"negation keeps the rest", "on:\n  push:\n    branches: ['releases/**', '!releases/**-rc']", "refs/heads/releases/v2", true},
		{"a later pattern adds back", "on:\n  push:\n    branches: ['releases/**', '!releases/**-rc', 'releases/v9-rc']", "refs/heads/releases/v9-rc", true},
		{"plus is one or more", "on:\n  push:\n    branches: ['v2+']", "refs/heads/v222", true},
		{"plus needs one", "on:\n  push:\n    branches: ['v2+']", "refs/heads/v", false},
		{"plus leaves others out", "on:\n  push:\n    branches: ['v2+']", "refs/heads/main", false},
		{"question mark is zero or one", "on:\n  push:\n    branches: ['mains?']", "refs/heads/main", true},
		{"question mark is not any character", "on:\n  push:\n    branches: ['mains?']", "refs/heads/mainx", false},
		{"class with plus", "on:\n  push:\n    tags: ['v[0-9]+.[0-9]+']", "refs/tags/v10.2", true},
		{"ignore with a quantifier", "on:\n  push:\n    branches-ignore: ['release-v[0-9]+']", "refs/heads/release-v12", false},
		{"star stays inside a segment", "on:\n  push:\n    branches: ['feature/*']", "refs/heads/feature/a/b", false},
		{"a malformed pattern runs", "on:\n  push:\n    branches: ['+oops']", "refs/heads/main", true},
		{"unknown ref runs", "on:\n  push:\n    tags: ['v*']", "", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := parseOn(t, c.on).PushMatchesRef(c.ref); got != c.want {
				t.Errorf("PushMatchesRef(%q) = %v, want %v", c.ref, got, c.want)
			}
		})
	}
}
