package expr

import (
	"errors"
	"testing"
)

func TestSplatOverAnObject(t *testing.T) {
	t.Parallel()

	ctx := Context{"fruits": map[string]any{
		"apple":  map[string]any{"color": "red"},
		"banana": map[string]any{"color": "yellow"},
	}}

	got, err := EvalCondition("contains(fruits.*.color, 'red')", ctx)
	if err != nil || !got {
		t.Errorf("contains over an object splat = %v (err=%v), want true", got, err)
	}

	first, err := EvalString("join(fruits.*.color, ',')", ctx)
	if err != nil {
		t.Fatal(err)
	}

	if first != "red,yellow" {
		t.Errorf("join over an object splat = %q, want sorted key order", first)
	}
}

func TestVarsAreUndecidable(t *testing.T) {
	t.Parallel()

	_, err := EvalCondition("vars.DEPLOY == 'true'", Context{})

	var ue *UndecidableError
	if !errors.As(err, &ue) {
		t.Fatalf("vars gate = %v, want UndecidableError", err)
	}
}

func TestCompositeContainingUnknownRefusesWholeUse(t *testing.T) {
	t.Parallel()

	ctx := Context{"inputs": map[string]any{
		"target": Unknown("inputs.target"),
		"ok":     "fine",
	}}

	if _, err := EvalString("toJSON(inputs)", ctx); err == nil {
		t.Error("toJSON over a nested unknown produced output instead of refusing")
	}

	if got, err := EvalString("inputs.ok", ctx); err != nil || got != "fine" {
		t.Errorf("sibling of an unknown = %q (err=%v), want fine", got, err)
	}
}

func TestAnUnmodelledContextIsUndecidable(t *testing.T) {
	t.Parallel()

	ctx := Context{"github": map[string]any{"ref": "refs/heads/main"}}

	for _, src := range []string{
		"future_context.enabled == 'yes'",
		"deployments.production.url != ''",
		"organization.name == 'acme'",
	} {
		if _, err := EvalCondition(src, ctx); err == nil {
			t.Errorf("EvalCondition(%q) answered a gate on a context it does not model", src)
		}
	}

	ok, err := EvalCondition("github.ref == 'refs/heads/main'", ctx)
	if err != nil || !ok {
		t.Errorf("a modelled context stopped resolving: %v %v", ok, err)
	}
}
