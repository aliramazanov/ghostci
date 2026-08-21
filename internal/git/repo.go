package git

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

func IsRepo(dir string) bool {
	_, err := Run(dir, cmdGitDir...)

	return err == nil
}

func Root(dir string) (string, error) {
	return Run(dir, cmdRoot...)
}

func Resolve(dir, rev string) (string, error) {
	return Run(dir, cmdVerify(rev+"^{commit}")...)
}

func SameCommit(dir, a, b string) bool {
	ra, err1 := Resolve(dir, a)
	rb, err2 := Resolve(dir, b)

	return err1 == nil && err2 == nil && ra == rb
}

func HooksDir(dir string) (string, error) { return Run(dir, cmdHooksPath...) }

func HooksPathConfig(dir string) (string, error) { return Run(dir, cmdHooksConfig...) }

const minVersion = "2.22"

var versionPattern = regexp.MustCompile(`\d+\.\d+`)

func CheckVersion(dir string) error {
	out, err := Run(dir, cmdVersion...)
	if err != nil {
		return nil
	}

	found := versionPattern.FindString(out)
	if found == "" {
		return nil
	}

	if compareVersions(found, minVersion) < 0 {
		return fmt.Errorf("git: %s is older than the required %s", found, minVersion)
	}

	return nil
}

func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")

	for i := range max(len(as), len(bs)) {
		an, bn := segment(as, i), segment(bs, i)
		if an != bn {
			return an - bn
		}
	}

	return 0
}

func segment(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}

	n, err := strconv.Atoi(parts[i])
	if err != nil {
		return 0
	}

	return n
}

func CommonDir(dir string) (string, error) { return Run(dir, cmdCommonDir...) }
