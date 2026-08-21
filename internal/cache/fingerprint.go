package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/aliramazanov/ghostci/internal/config"
)

const Version = "1"

type Fingerprint struct {
	Version    string            `json:"version"`
	Command    string            `json:"command"`
	Dir        string            `json:"dir,omitempty"`
	Shell      string            `json:"shell,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	Inputs     map[string]string `json:"inputs"`
	Toolchains map[string]string `json:"toolchains,omitempty"`
}

func New(chk config.Check, inputs, toolchains map[string]string) Fingerprint {
	return Fingerprint{
		Version:    Version,
		Command:    chk.Command,
		Dir:        chk.Dir,
		Shell:      chk.Shell,
		Env:        chk.Env,
		Inputs:     inputs,
		Toolchains: toolchains,
	}
}

func (f Fingerprint) Key() (string, error) {
	body, err := json.Marshal(f)

	if err != nil {
		return "", fmt.Errorf("cache: serialising fingerprint: %w", err)
	}

	sum := sha256.Sum256(body)

	return hex.EncodeToString(sum[:]), nil
}

func (f Fingerprint) Diff(other Fingerprint) []string {
	var out []string

	if f.Version != other.Version {
		out = append(out, fmt.Sprintf("ghostci version %s -> %s", other.Version, f.Version))
	}

	if f.Command != other.Command {
		out = append(out, "command changed")
	}

	if f.Dir != other.Dir {
		out = append(out, "working directory changed")
	}

	if f.Shell != other.Shell {
		out = append(out, "shell changed")
	}

	out = append(out, diffMaps("env", f.Env, other.Env)...)
	out = append(out, diffMaps("toolchain", f.Toolchains, other.Toolchains)...)
	out = append(out, diffMaps("", f.Inputs, other.Inputs)...)

	return out
}

func diffMaps(label string, now, before map[string]string) []string {
	var changed, added, removed []string

	for k, v := range now {
		old, ok := before[k]
		switch {
		case !ok:
			added = append(added, k)
		case old != v:
			changed = append(changed, k)
		}
	}

	for k := range before {
		if _, ok := now[k]; !ok {
			removed = append(removed, k)
		}
	}

	sort.Strings(changed)
	sort.Strings(added)
	sort.Strings(removed)

	var out []string
	prefix := ""

	if label != "" {
		prefix = label + " "
	}

	if len(changed) > 0 {
		out = append(out, fmt.Sprintf("%schanged: %s", prefix, summarise(changed)))
	}

	if len(added) > 0 {
		out = append(out, fmt.Sprintf("%sadded: %s", prefix, summarise(added)))
	}

	if len(removed) > 0 {
		out = append(out, fmt.Sprintf("%sremoved: %s", prefix, summarise(removed)))
	}

	return out
}

func summarise(items []string) string {
	const max = 3

	if len(items) <= max {
		return strings.Join(items, ", ")
	}

	return fmt.Sprintf("%s and %d more", strings.Join(items[:max], ", "), len(items)-max)
}
