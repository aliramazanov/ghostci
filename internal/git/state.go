package git

import (
	"errors"
	"strings"
)

func Dirty(dir string) ([]string, error) {
	out, err := Raw(dir, cmdStatus...)
	if err != nil {
		return nil, err
	}

	var paths []string
	records := strings.Split(out, "\x00")

	for i := 0; i < len(records); i++ {
		rec := records[i]
		if len(rec) < 4 {
			continue
		}

		status, path := rec[:2], rec[3:]
		if path != "" {
			paths = append(paths, path)
		}

		if status[0] == 'R' || status[0] == 'C' {
			i++
			if i < len(records) && records[i] != "" {
				paths = append(paths, records[i])
			}
		}
	}

	return paths, nil
}

func Blobs(dir string) (map[string]string, error) {
	out, err := Raw(dir, cmdIndex...)
	if err != nil {
		return nil, err
	}

	blobs := map[string]string{}

	for _, rec := range strings.Split(out, "\x00") {
		meta, path, ok := strings.Cut(rec, "\t")
		if !ok {
			continue
		}

		fields := strings.Fields(meta)
		if len(fields) < 2 {
			continue
		}

		blobs[path] = fields[1]
	}

	return blobs, nil
}

var ErrBadRevision = errors.New("git: a revision may not begin with -")

func Changed(dir, base string) ([]string, error) {
	if strings.HasPrefix(base, "-") {
		return nil, ErrBadRevision
	}

	out, err := Raw(dir, cmdDiff(base+"...HEAD")...)
	if err != nil {
		out, err = Raw(dir, cmdDiff(base)...)
	}
	if err != nil {
		return nil, err
	}

	var paths []string

	for _, rec := range strings.Split(out, "\x00") {
		if rec != "" {
			paths = append(paths, rec)
		}
	}

	return paths, nil
}
