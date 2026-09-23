package importer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCorpusExtractionRate(t *testing.T) {
	t.Parallel()
	repos, err := os.ReadDir(filepath.Join("testdata", "corpus"))
	if err != nil {
		t.Fatal(err)
	}

	var extracted, runSteps, files int
	for _, r := range repos {
		if !r.IsDir() {
			continue
		}
		dir := filepath.Join("testdata", "corpus", r.Name(), ".github", "workflows")
		matches, err := filepath.Glob(filepath.Join(dir, "*.y*ml"))
		if err != nil {
			t.Fatal(err)
		}
		files += len(matches)

		res, err := ImportDir(dir, DefaultAssumptions())
		if err != nil {
			t.Fatalf("%s: %v", r.Name(), err)
		}
		e, n, _ := res.ExtractionRate()
		extracted += e
		runSteps += n
	}

	if files < 120 || runSteps < 500 {
		t.Fatalf("corpus too small to mean anything: %d files, %d run steps", files, runSteps)
	}

	pct := float64(extracted) * 100 / float64(runSteps)
	t.Logf("corpus: %d files, %d/%d run steps extracted (%.1f%%)", files, extracted, runSteps, pct)
	if pct < 70 {
		t.Errorf("extraction rate fell to %.1f%%, the floor is 70%%", pct)
	}
}
