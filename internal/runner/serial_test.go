package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aliramazanov/ghostci/internal/config"
)

func TestSerialChecksNeverOverlap(t *testing.T) {
	log := filepath.Join(t.TempDir(), "log")

	checks := make([]config.Check, 0, 8)
	for i := 0; i < 6; i++ {
		checks = append(checks, config.Check{
			Name:    "installer",
			Command: "echo start >> " + log + "; sleep 0.05; echo end >> " + log,
			Serial:  true,
		})
	}

	for i := 0; i < 2; i++ {
		checks = append(checks, config.Check{Name: "free", Command: "true"})
	}

	results := Run(context.Background(), checks, Options{Jobs: 8})
	for _, r := range results {
		if r.Status != StatusPassed {
			t.Fatalf("%s: %v\n%s", r.Name, r.Status, r.Output)
		}
	}

	body, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}

	var open int
	for _, line := range strings.Fields(string(body)) {
		switch line {
		case "start":
			open++
			if open > 1 {
				t.Fatalf("two serial checks ran at once:\n%s", body)
			}
		case "end":
			open--
		}
	}
}
