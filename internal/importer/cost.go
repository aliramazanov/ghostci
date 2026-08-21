package importer

import "strings"

type Cost int

const (
	CostCheck Cost = iota
	CostHeavy
)

var heavyJobMarkers = []string{
	"release", "publish", "deploy", "package", "binaries", "artifact",
	"docker", "image", "installer", "bundle", "dist", "upload", "sign",
	"benchmark", "bench", "fuzz", "nightly", "codeql", "e2e",
}

var heavyCommandMarkers = []string{
	"docker build", "docker push", "buildx",
	"npm publish", "yarn publish", "pnpm publish", "cargo publish", "twine upload",
	"maturin build", "python -m build", "setup.py sdist", "bdist_wheel",
	"cargo build --release", "goreleaser", "electron-builder",
	"codesign", "notarize", "cross build",
	"--profile-generate", "--profile-use", "profile-use", "pgo",
	"gh release", "aws s3", "gsutil", "kubectl apply", "terraform apply",
	"helm upgrade", "cargo bench", "go test -bench",
}

var checkCommandMarkers = []string{
	"lint", "fmt", "vet", "clippy", "eslint", "prettier", "biome",
	"typecheck", "tsc ", "mypy", "ruff check", "flake8", "pylint",
	"test", "pytest", "spec", "shellcheck", "audit", "check",
}

var containerCommands = []string{"docker run", "docker build", "docker compose", "podman run", "podman build"}

func classifyCost(jobID, command string) (Cost, string) {
	lowerCmd := strings.ToLower(command)

	for _, m := range containerCommands {
		if strings.Contains(lowerCmd, m) {
			return CostHeavy, "needs a container runtime (" + m + ")"
		}
	}

	for _, m := range heavyCommandMarkers {
		if strings.Contains(lowerCmd, m) {
			return CostHeavy, "builds or ships artifacts (" + m + ")"
		}
	}
	for _, m := range checkCommandMarkers {
		if strings.Contains(lowerCmd, m) {
			return CostCheck, ""
		}
	}

	lowerJob := strings.ToLower(jobID)
	for _, m := range heavyJobMarkers {
		if strings.Contains(lowerJob, m) {
			return CostHeavy, "part of the " + jobID + " job, which produces artifacts"
		}
	}
	return CostCheck, ""
}
