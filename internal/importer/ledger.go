package importer

import "github.com/aliramazanov/ghostci/internal/pipeline"

type ledger struct {
	res   *Result
	label string
}

func (l *ledger) refuse(job string, r pipeline.Refusal, reason string) {
	l.res.Entries = append(l.res.Entries, Entry{
		Workflow: l.label, Job: job, Step: "step",
		Outcome: refusalOutcome(r), Reason: reason,
	})
}

func (l *ledger) note(job, text string) {
	l.res.Warnings = append(l.res.Warnings, job+": "+text)
}

func (l *ledger) recordToolchainFrom(image, source string) {
	tool, version := imageToolchain(image)
	if tool == "" {
		return
	}

	for _, t := range l.res.Toolchains {
		if t.Name == tool {
			return
		}
	}

	l.res.Toolchains = append(l.res.Toolchains,
		Toolchain{Name: tool, Version: version, Source: source + image})
}
