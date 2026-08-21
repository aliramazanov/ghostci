package expr

import "testing"

func fuzzContext() Context {
	return Context{
		"github": map[string]any{
			"ref":        "refs/heads/main",
			"event_name": "push",
			"repository": "o/r",
		},
		"env":    map[string]any{"CI": "true"},
		"inputs": map[string]any{"x": Unknown("inputs.x")},
		"matrix": map[string]any{"os": "linux", "list": []any{"a", "b"}},
	}
}

func FuzzEvalCondition(f *testing.F) {
	for _, seed := range []string{
		"github.ref == 'refs/heads/main'",
		"startsWith(github.ref, 'refs/tags/')",
		"!contains(fromJSON('[\"a\"]'), matrix.os)",
		"matrix.list.*.name",
		"env.CI && (1 < 2) || failure()",
		"format('{0}-{1}', github.ref, matrix.os)",
		"a[b[c[0]]]",
		"'un\\'terminated",
		"((((((((((1))))))))))",
		"0x10 == 16 && -1.5e3 < 0",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, src string) {
		EvalCondition(src, fuzzContext())
	})
}

func FuzzInterpolate(f *testing.F) {
	for _, seed := range []string{
		"echo ${{ github.ref }}",
		"${{ matrix.os }}-${{ env.CI }}",
		"${{ toJSON(matrix) }}",
		"${{ ${{ nested }} }}",
		"${{ unclosed",
		"}} ${{",
		"${{ join(matrix.list, ',') }}",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, src string) {
		Interpolate(src, fuzzContext())
	})
}
