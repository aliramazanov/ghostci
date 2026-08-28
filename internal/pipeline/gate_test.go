package pipeline

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestUndecidableOutranksFalse(t *testing.T) {
	t.Parallel()

	gate := func(event string) (bool, error) {
		if event == "push" {
			return false, nil
		}

		return false, Unknown("$CI_MERGE_REQUEST_IID").Err()
	}

	verdict, passing, err := Decide([]string{"push", "merge_request"}, gate)
	if verdict != Undecidable {
		t.Fatalf("verdict = %v, want Undecidable", verdict)
	}
	if len(passing) != 0 {
		t.Errorf("passing = %v, want none", passing)
	}

	var ue *UndecidableError
	if !errors.As(err, &ue) {
		t.Errorf("err = %v, want an UndecidableError", err)
	}
}

func TestOneYesIsEnough(t *testing.T) {
	t.Parallel()

	gate := func(event string) (bool, error) {
		if event == "push" {
			return true, nil
		}

		return false, Unknown("$CI_MERGE_REQUEST_IID").Err()
	}

	verdict, passing, err := Decide([]string{"push", "merge_request"}, gate)
	if verdict != Runs || err != nil {
		t.Fatalf("verdict = %v, err = %v, want Runs", verdict, err)
	}
	if len(passing) != 1 || passing[0] != "push" {
		t.Errorf("passing = %v, want [push]", passing)
	}
}

func TestEveryEventSayingNoIsASkip(t *testing.T) {
	t.Parallel()

	verdict, _, err := Decide([]string{"push", "merge_request"},
		func(string) (bool, error) { return false, nil })

	if verdict != Skipped || err != nil {
		t.Fatalf("verdict = %v, err = %v, want Skipped", verdict, err)
	}
}

func TestValueSemantics(t *testing.T) {
	t.Parallel()

	if !Known("x").Truthy() {
		t.Error("a set, non-empty value is truthy")
	}
	if Known("").Truthy() {
		t.Error("an empty value is not truthy")
	}
	if Undefined().Truthy() {
		t.Error("an unset value is not truthy")
	}
	if Undefined().Err() != nil {
		t.Error("unset is decidably absent, not undecidable")
	}

	err := Unknown("vars.TOKEN").Err()
	if err == nil || !strings.Contains(err.Error(), "vars.TOKEN") {
		t.Errorf("err = %v, want it to name the unresolved source", err)
	}
}

func TestGateReasonNamesTheCondition(t *testing.T) {
	t.Parallel()

	got := GateReason("rule", "$CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH", Unknown("$CI_DEFAULT_BRANCH").Err())

	for _, want := range []string{"rule", "$CI_DEFAULT_BRANCH", "only known inside CI"} {
		if !strings.Contains(got, want) {
			t.Errorf("reason %q does not contain %q", got, want)
		}
	}
}

func TestCartesianKeepsEveryAxisKey(t *testing.T) {
	t.Parallel()

	axes := []Axis{
		{Key: "os", Values: []any{"linux", "mac", "windows"}},
		{Key: "go", Values: []any{"1.21", "1.22"}},
	}

	combos, truncated := Cartesian(axes)
	if truncated {
		t.Fatal("six combinations is not a truncation")
	}
	if len(combos) != 6 {
		t.Fatalf("got %d combinations, want 6", len(combos))
	}
	for _, c := range combos {
		if _, ok := c["os"]; !ok {
			t.Fatalf("combination missing os: %v", c)
		}
		if _, ok := c["go"]; !ok {
			t.Fatalf("combination missing go: %v", c)
		}
	}
}

func TestTruncationStillCarriesLaterAxes(t *testing.T) {
	t.Parallel()

	big := make([]any, 40)
	for i := range big {
		big[i] = i
	}

	combos, truncated := Cartesian([]Axis{
		{Key: "a", Values: big},
		{Key: "b", Values: big},
		{Key: "c", Values: []any{"only"}},
	})

	if !truncated {
		t.Fatal("1600 combinations should report truncation")
	}
	for _, c := range combos {
		if c["c"] != "only" {
			t.Fatalf("a leg lost the axis after the overflow: %v", c)
		}
	}
}

func TestCombinationMatchesPartially(t *testing.T) {
	t.Parallel()

	c := Combination{"os": "linux", "go": "1.22"}

	if !c.Matches(Combination{"os": "linux"}) {
		t.Error("a partial filter naming a matching key should match")
	}
	if c.Matches(Combination{"os": "mac"}) {
		t.Error("a filter naming a different value should not match")
	}
	if c.Matches(Combination{"absent": "x"}) {
		t.Error("a filter naming an absent key should not match")
	}
	if !c.Matches(Combination{"go": 1.22}) && !c.Matches(Combination{"go": "1.22"}) {
		t.Error("values spelled as different YAML types should still compare")
	}
}

func TestTrimCutsOnRuneBoundaries(t *testing.T) {
	t.Parallel()

	for _, r := range []string{"é", "🙂", "日", "a"} {
		got := Trim(strings.Repeat(r, 60))
		if !utf8.ValidString(got) {
			t.Errorf("Trim of %q characters produced invalid UTF-8: %q", r, got)
		}
		if len(got) > 60 {
			t.Errorf("Trim of %q returned %d bytes, want at most 60", r, len(got))
		}
	}
}
