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
		{"negation is undecided, so it runs", "on:\n  push:\n    branches: ['releases/**', '!releases/**-rc']", "refs/heads/main", true},
		{"quantifier is undecided, so it runs", "on:\n  push:\n    branches: ['v2+']", "refs/heads/main", true},
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
