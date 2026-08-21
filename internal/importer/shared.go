package importer

import (
	"sort"
	"strconv"
	"time"

	"github.com/aliramazanov/ghostci/internal/config"
)

func routeCheck(res *Result, job string, chk config.Check) bool {
	cost, why := classifyCost(job, chk.Command)
	if cost != CostHeavy {
		res.Checks = append(res.Checks, chk)

		return false
	}

	if res.HeavyReason == nil {
		res.HeavyReason = map[string]string{}
	}
	res.HeavyReason[chk.Name] = why
	res.Heavy = append(res.Heavy, chk)

	return true
}

func mergeStringMaps(layers ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, layer := range layers {
		for k, v := range layer {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}

	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)

	return out
}

func itoa(i int) string { return strconv.Itoa(i) }

const maxImportedTimeout = 30 * 24 * time.Hour

func timeoutMinutes(n int) time.Duration {
	if n <= 0 {
		return 0
	}

	if scaled := time.Duration(n) * time.Minute; scaled > 0 && scaled < maxImportedTimeout {
		return scaled
	}

	return maxImportedTimeout
}
