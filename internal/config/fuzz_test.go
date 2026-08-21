package config

import (
	"testing"
	"time"
)

func FuzzParse(f *testing.F) {
	for _, seed := range []string{
		"checks:\n  - name: a\n    command: b\n",
		"checks:\n  - name: a\n    command: b\n    timeout: 90s\n    inputs: ['**/*.go']\n",
		"checks: []\n",
		"checks:\n  - name: a\n    command: b\n    inputs: ['[bad']\n",
		"checks:\n  - {}\n",
		"checks:\n  - name: a\n    command: b\n    timeout: notaduration\n",
		"checks:\n  - name: dup\n    command: x\n  - name: dup\n    command: y\n",
		"not a mapping",
		"checks:\n  - name: a\n    command: b\n    env: {A: 1}\n",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, src string) {
		cfg, err := Parse([]byte(src))
		if err != nil {
			return
		}
		if cfg == nil {
			t.Fatal("Parse returned no config and no error")
		}

		for _, chk := range cfg.Checks {
			if chk.Name == "" {
				t.Errorf("accepted a check with no name: %q", src)
			}
			if chk.Command == "" {
				t.Errorf("accepted a check with no command: %q", src)
			}
			if time.Duration(chk.Timeout) < 0 {
				t.Errorf("accepted a negative timeout, which is already expired: %q", src)
			}

			if len(chk.Inputs) == 0 && !chk.AlwaysRuns() {
				t.Errorf("a check with no inputs is not always-running: %q", src)
			}
		}
	})
}
