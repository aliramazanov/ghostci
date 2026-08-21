package config

import (
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

func TestConfigRoundTripsThroughYAML(t *testing.T) {
	t.Parallel()

	want := Config{Checks: []Check{{
		Name:     "test",
		Command:  "go test ./...",
		Timeout:  Duration(90 * time.Second),
		Inputs:   []string{"**/*.go"},
		Env:      map[string]string{"CI": "true"},
		Serial:   true,
		Optional: true,
	}}}

	body, err := yaml.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "1m30s") {
		t.Errorf("timeout was not written as a duration string:\n%s", body)
	}

	var got Config
	if err := yaml.Unmarshal(body, &got); err != nil {
		t.Fatalf("ghostci cannot read back what it wrote: %v\n%s", err, body)
	}

	if len(got.Checks) != 1 {
		t.Fatalf("got %d checks", len(got.Checks))
	}
	if time.Duration(got.Checks[0].Timeout) != 90*time.Second {
		t.Errorf("timeout = %v, want 90s", time.Duration(got.Checks[0].Timeout))
	}
	if !got.Checks[0].Serial || !got.Checks[0].Optional {
		t.Errorf("flags did not survive: %+v", got.Checks[0])
	}
}
