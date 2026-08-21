package importer

import "testing"

func TestToolchainVersionFromTheRef(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
name: ci
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: dtolnay/rust-toolchain@1.75
      - run: cargo test
`)

	if len(res.Toolchains) != 1 {
		t.Fatalf("toolchains = %v, want one", res.Toolchains)
	}
	if got := res.Toolchains[0].Version; got != "1.75" {
		t.Errorf("version = %q, want 1.75", got)
	}
	if got := res.Toolchains[0].Name; got != "rust-toolchain" {
		t.Errorf("name = %q", got)
	}
}

func TestSetupActionRefIsNotAToolchainVersion(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
name: ci
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/setup-go@v5
      - run: go test ./...
`)

	if len(res.Toolchains) != 0 {
		t.Errorf("toolchains = %v, want none: v5 is the action version", res.Toolchains)
	}
}
