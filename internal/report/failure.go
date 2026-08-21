package report

import (
	"fmt"
	"io"
	"strings"
)

const rule = 60

type Failure struct {
	Name   string
	Output string
}

func (f Failure) WriteTo(w io.Writer) (int64, error) {
	body := trimBlank(f.Output)
	if len(body) == 0 {
		return 0, nil
	}

	n, err := fmt.Fprintf(w, "\n  %s %s\n", header(f.Name), divider(f.Name))
	if err != nil {
		return int64(n), err
	}

	for _, line := range body {
		m, err := fmt.Fprintf(w, "  %s\n", line)
		n += m

		if err != nil {
			return int64(n), err
		}
	}

	return int64(n), nil
}

func header(name string) string { return "── " + name }

func divider(name string) string {
	if pad := rule - len(name); pad > 0 {
		return strings.Repeat("─", pad)
	}

	return ""
}

func trimBlank(out string) []string {
	var kept []string

	blank := true

	for _, line := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			if blank {
				continue
			}

			blank = true
			kept = append(kept, "")

			continue
		}

		blank = false
		kept = append(kept, strings.TrimRight(line, " \t"))
	}

	for len(kept) > 0 && kept[len(kept)-1] == "" {
		kept = kept[:len(kept)-1]
	}

	return kept
}
