// Package importer classifies imported CI jobs by execution cost.
package importer

import "strings"

type Cost int

const (
	CostCheck Cost = iota
	CostHeavy
)

var sideEffectMarkers = []string{
	"git push", "git commit", "git tag", "git am",
	"git checkout", "git reset", "git clean", "git rebase", "git merge",
	"git switch", "git restore", "git stash", "git cherry-pick", "git revert",
	"git fetch", "git pull", "git submodule",
	"gh release", "gh pr ", "gh issue ", "gh api ", "gh workflow run",
	"goveralls", "coveralls", "codecov", "codacy", "sonar-scanner", "sonarqube",
	"npm publish", "yarn publish", "pnpm publish", "cargo publish",
	"twine upload", "poetry publish", "gem push", "mvn deploy",
	"gradle publish", "dotnet nuget push",
	"vercel deploy", "netlify deploy", "firebase deploy", "fly deploy",
	"aws s3", "gsutil", "az storage", "kubectl apply", "terraform apply",
	"helm upgrade", "docker push",
	"scp ", "rsync ", "ssh ",

	"apt-get install", "apt install", "yum install", "dnf install",
	"apk add", "pacman -s", "brew install", "choco install", "snap install",
	"systemctl", "setcap", "update-alternatives",

	"cargo install", "go install", "pipx install", "gem install",
	"npm i -g", "npm install -g", "npm install --global",
	"pnpm add -g", "yarn global add", "pip install --user",
}

func postsSomewhere(lower string) bool {
	for _, stmt := range statements(lower) {
		if postsInStatement(stmt) {
			return true
		}
	}

	return false
}

func postsInStatement(stmt string) bool {
	if !strings.Contains(stmt, "curl") && !strings.Contains(stmt, "wget") {
		return false
	}

	writes := strings.Contains(stmt, "-x post") || strings.Contains(stmt, "-x put") ||
		strings.Contains(stmt, "--data") || strings.Contains(stmt, " -d ") ||
		strings.Contains(stmt, "--post")
	if !writes {
		return false
	}

	for _, local := range []string{"localhost", "127.0.0.1", "0.0.0.0", "[::1]"} {
		if strings.Contains(stmt, local) {
			return false
		}
	}

	return true
}

func needsRoot(lower string) bool {
	for _, stmt := range statements(lower) {
		if fields := strings.Fields(stmt); len(fields) > 0 && fields[0] == "sudo" {
			return true
		}
	}

	return false
}

func statements(script string) []string {
	return strings.FieldsFunc(script, func(r rune) bool {
		return r == '\n' || r == ';' || r == '|' || r == '&'
	})
}

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

	if needsRoot(lowerCmd) {
		return CostHeavy, "needs root, so it changes the machine and not just this repository"
	}

	for _, m := range sideEffectMarkers {
		if strings.Contains(lowerCmd, m) {
			return CostHeavy, "changes something outside this machine (" + strings.TrimSpace(m) + ")"
		}
	}

	if postsSomewhere(lowerCmd) {
		return CostHeavy, "sends data to a remote host"
	}

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
