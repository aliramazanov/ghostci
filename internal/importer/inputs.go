package importer

import "github.com/aliramazanov/ghostci/internal/shell"

var inputRules = []struct {
	tools []string
	globs []string
}{
	{[]string{"go", "gofmt", "gofumpt", "golangci-lint", "staticcheck", "goimports", "govulncheck"},
		[]string{"**/*.go", "go.mod", "go.sum"}},

	{[]string{"cargo", "rustc", "rustfmt", "clippy-driver"},
		[]string{"**/*.rs", "Cargo.toml", "Cargo.lock", "rust-toolchain.toml"}},

	{[]string{"eslint", "prettier", "tsc", "vitest", "jest", "biome", "oxlint", "rollup", "vite", "webpack", "next", "swc",
		"node", "deno", "tsx", "ts-node"},
		[]string{"**/*.js", "**/*.jsx", "**/*.mjs", "**/*.cjs", "**/*.ts", "**/*.tsx",
			"package.json", "tsconfig*.json", "**/*.svelte", "**/*.vue"}},

	{[]string{"pytest", "mypy", "ruff", "pylint", "flake8", "black", "isort", "tox", "nox", "pyright",
		"python", "python3", "pip", "pip3", "uv"},
		[]string{"**/*.py", "**/*.pyi", "pyproject.toml", "setup.cfg", "setup.py",
			"requirements*.txt", "tox.ini"}},

	{[]string{"mvn", "gradle", "gradlew", "javac", "ktlint", "spotless"},
		[]string{"**/*.java", "**/*.kt", "**/*.kts", "pom.xml", "build.gradle*", "settings.gradle*"}},

	{[]string{"shellcheck", "shfmt", "bashate"}, []string{"**/*.sh", "**/*.bash", "**/*.zsh"}},
	{[]string{"terraform", "tflint", "tofu"}, []string{"**/*.tf", "**/*.tfvars", "**/*.hcl"}},
	{[]string{"hadolint"}, []string{"**/Dockerfile*", "**/*.dockerfile"}},
	{[]string{"yamllint"}, []string{"**/*.yml", "**/*.yaml"}},
	{[]string{"actionlint"}, []string{".github/workflows/**"}},
	{[]string{"markdownlint", "markdownlint-cli2", "mdformat"}, []string{"**/*.md", "**/*.mdx"}},
	{[]string{"stylelint"}, []string{"**/*.css", "**/*.scss", "**/*.less"}},
	{[]string{"dotnet", "msbuild"}, []string{"**/*.cs", "**/*.csproj", "**/*.sln"}},
	{[]string{"swift", "swiftlint"}, []string{"**/*.swift", "Package.swift"}},
	{[]string{"rubocop", "rspec", "bundle"}, []string{"**/*.rb", "Gemfile", "Gemfile.lock", "*.gemspec"}},
	{[]string{"php", "phpunit", "phpstan", "psalm"}, []string{"**/*.php", "composer.json", "composer.lock"}},

	{[]string{"cmake", "ninja", "gcc", "g++", "cc", "clang", "clang++"}, []string{"**/*.c", "**/*.h", "**/*.cc", "**/*.cpp", "**/*.hpp",
		"CMakeLists.txt", "**/Makefile", "**/*.mk"}},
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

	for _, rule := range inputRules {
		matched := false
		for _, tool := range rule.tools {
			if invoked[tool] {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}

		add(rule.globs)
	}

	if depth < maxRecipeDepth {
		if inner, ok := scriptInputs(root, dir, command, depth); ok {
			add(inner)
		}
	}

	if invoked["make"] && depth < maxRecipeDepth {
		goals, at := makeGoals(command)
		if at == "" {
			at = dir
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
