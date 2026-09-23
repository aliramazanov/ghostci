package importer

import (
	"path"
	"slices"
	"strings"

	"github.com/aliramazanov/ghostci/internal/shell"
)

var inputRules = []struct {
	tools []string
	globs []string
}{
	{[]string{"go", "gofmt", "gofumpt", "golangci-lint", "staticcheck", "goimports", "govulncheck"},
		[]string{"**/*.go", "go.mod", "go.sum", "go.work", "go.work.sum"}},

	{[]string{"cargo", "rustc", "rustfmt", "clippy-driver"},
		[]string{"**/*.rs", "Cargo.toml", "Cargo.lock", "rust-toolchain.toml"}},

	{[]string{"eslint", "prettier", "tsc", "vitest", "jest", "biome", "oxlint", "rollup", "vite", "webpack", "next", "swc",
		"node", "deno", "tsx", "ts-node"},
		[]string{"**/*.js", "**/*.jsx", "**/*.mjs", "**/*.cjs", "**/*.ts", "**/*.tsx",
			"package.json", "tsconfig*.json", "**/*.svelte", "**/*.vue",
			"package-lock.json", "npm-shrinkwrap.json", "yarn.lock", "pnpm-lock.yaml", "bun.lockb", "bun.lock"}},

	{[]string{"pytest", "mypy", "ruff", "pylint", "flake8", "black", "isort", "tox", "nox", "pyright",
		"python", "python3", "pip", "pip3", "uv"},
		[]string{"**/*.py", "**/*.pyi", "pyproject.toml", "setup.cfg", "setup.py",
			"requirements*.txt", "tox.ini", "uv.lock", "poetry.lock", "pdm.lock", "Pipfile", "Pipfile.lock"}},

	{[]string{"mvn", "gradle", "gradlew", "javac", "ktlint", "spotless"},
		[]string{"**/*.java", "**/*.kt", "**/*.kts", "pom.xml", "build.gradle*", "settings.gradle*",
			"gradle.properties", "gradle/libs.versions.toml", "gradle/wrapper/gradle-wrapper.properties"}},

	{[]string{"shellcheck", "shfmt", "bashate"}, []string{"**/*.sh", "**/*.bash", "**/*.zsh"}},
	{[]string{"terraform", "tflint", "tofu"}, []string{"**/*.tf", "**/*.tfvars", "**/*.hcl"}},
	{[]string{"hadolint"}, []string{"**/Dockerfile*", "**/*.dockerfile"}},
	{[]string{"yamllint"}, []string{"**/*.yml", "**/*.yaml"}},
	{[]string{"actionlint"}, []string{".github/workflows/**"}},
	{[]string{"markdownlint", "markdownlint-cli2", "mdformat"}, []string{"**/*.md", "**/*.mdx"}},
	{[]string{"stylelint"}, []string{"**/*.css", "**/*.scss", "**/*.less"}},
	{[]string{"dotnet", "msbuild"}, []string{"**/*.cs", "**/*.csproj", "**/*.sln",
		"**/packages.lock.json", "Directory.Packages.props", "Directory.Build.props", "global.json"}},
	{[]string{"swift", "swiftlint"}, []string{"**/*.swift", "Package.swift", "Package.resolved"}},
	{[]string{"rubocop", "rspec", "bundle"}, []string{"**/*.rb", "Gemfile", "Gemfile.lock", "*.gemspec"}},
	{[]string{"php", "phpunit", "phpstan", "psalm"}, []string{"**/*.php", "composer.json", "composer.lock"}},

	{[]string{"cmake", "ninja", "gcc", "g++", "cc", "clang", "clang++"}, []string{"**/*.c", "**/*.h", "**/*.cc", "**/*.cpp", "**/*.hpp",
		"CMakeLists.txt", "**/Makefile", "**/*.mk"}},

	{[]string{"golangci-lint"}, []string{"**/.golangci.yml", "**/.golangci.yaml", "**/.golangci.toml", "**/.golangci.json"}},
	{[]string{"staticcheck"}, []string{"**/staticcheck.conf"}},
	{[]string{"cargo", "rustfmt", "clippy-driver"}, []string{"**/rustfmt.toml", "**/.rustfmt.toml",
		"**/clippy.toml", "**/.clippy.toml", "**/.cargo/config.toml", "**/.cargo/config"}},
	{[]string{"eslint"}, []string{"**/.eslintrc*", "**/eslint.config.*", "**/.eslintignore"}},
	{[]string{"prettier"}, []string{"**/.prettierrc*", "**/prettier.config.*", "**/.prettierignore", "**/.editorconfig"}},
	{[]string{"biome"}, []string{"**/biome.json", "**/biome.jsonc"}},
	{[]string{"oxlint"}, []string{"**/.oxlintrc.json"}},
	{[]string{"jest", "vitest"}, []string{"**/jest.config.*", "**/vitest.config.*", "**/vitest.workspace.*"}},
	{[]string{"ruff"}, []string{"**/ruff.toml", "**/.ruff.toml"}},
	{[]string{"mypy"}, []string{"**/mypy.ini", "**/.mypy.ini"}},
	{[]string{"flake8"}, []string{"**/.flake8"}},
	{[]string{"pylint"}, []string{"**/.pylintrc", "**/pylintrc"}},
	{[]string{"pytest"}, []string{"**/pytest.ini"}},
	{[]string{"pyright"}, []string{"**/pyrightconfig.json"}},
	{[]string{"isort"}, []string{"**/.isort.cfg"}},
	{[]string{"ktlint"}, []string{"**/.editorconfig"}},
	{[]string{"rubocop"}, []string{"**/.rubocop.yml"}},
	{[]string{"phpstan"}, []string{"**/phpstan.neon*"}},
	{[]string{"psalm"}, []string{"**/psalm.xml*"}},
	{[]string{"phpunit"}, []string{"**/phpunit.xml*"}},
	{[]string{"swiftlint"}, []string{"**/.swiftlint.yml"}},
	{[]string{"shellcheck"}, []string{"**/.shellcheckrc"}},
	{[]string{"yamllint"}, []string{"**/.yamllint*"}},
	{[]string{"markdownlint", "markdownlint-cli2"}, []string{"**/.markdownlint*"}},
	{[]string{"hadolint"}, []string{"**/.hadolint.yaml", "**/.hadolint.yml"}},
	{[]string{"stylelint"}, []string{"**/.stylelintrc*", "**/stylelint.config.*"}},
}

func inferInputs(command string) []string { return inferInputsIn("", "", command) }

func inferInputsIn(root, dir, command string) []string {
	return inferAt(root, dir, command, 0)
}

func inferAt(root, dir, command string, depth int) []string {
	invoked := map[string]bool{}
	for _, tool := range shell.Invoked(command) {
		invoked[tool] = true
	}

	if len(invoked) == 0 {
		return nil
	}

	moves := invoked["cd"] || invoked["pushd"]

	seen := map[string]bool{}
	var globs []string

	add := func(gs []string) {
		for _, g := range gs {
			if !seen[g] {
				seen[g] = true
				globs = append(globs, g)
			}
		}
	}

	for _, rule := range matchingRules(invoked) {
		if moves {
			add(anywhere(rule))
		} else {
			add(scoped(rule, dir))
		}
	}

	if moves {
		return globs
	}

	if depth < maxRecipeDepth {
		if inner, ok := scriptInputs(root, dir, command, depth); ok {
			add(inner)
		}
	}

	if invoked["make"] && depth < maxRecipeDepth {
		goals, at := makeGoals(command)
		switch {
		case at == "":
			at = dir
		case !path.IsAbs(at):
			at = path.Join(dir, at)
		}

		if recipe, ok := makeRecipe(root, at, goals); ok {
			if inner := inferAt(root, at, recipe, depth+1); len(inner) > 0 {
				add(inner)
				add([]string{"**/Makefile", "**/*.mk"})
			}
		}
	}

	return globs
}

func matchingRules(invoked map[string]bool) [][]string {
	var out [][]string

	for _, rule := range inputRules {
		if slices.ContainsFunc(rule.tools, func(tool string) bool { return invoked[tool] }) {
			out = append(out, rule.globs)
		}
	}

	return out
}

func scoped(globs []string, dirs ...string) []string {
	out := slices.Clone(globs)

	seen := make(map[string]bool, len(globs))
	for _, g := range globs {
		seen[g] = true
	}

	for _, dir := range dirs {
		base, ok := repoRelative(dir)
		if !ok {
			continue
		}

		for _, g := range globs {
			if strings.HasPrefix(g, "**/") {
				continue
			}

			if p := path.Join(base, g); !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}

	return out
}

func anywhere(globs []string) []string {
	out := slices.Clone(globs)

	for _, g := range globs {
		if !strings.HasPrefix(g, "**/") {
			out = append(out, "**/"+g)
		}
	}

	return out
}

func repoRelative(dir string) (string, bool) {
	if strings.ContainsAny(dir, `$~*?[]{}\`) {
		return "", false
	}

	clean := path.Clean(dir)
	if clean == "." || clean == ".." || path.IsAbs(clean) || strings.HasPrefix(clean, "../") {
		return "", false
	}

	return clean, true
}
