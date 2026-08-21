package pipeline

type Refusal int

const (
	NeedsContainer Refusal = iota

	OpaqueUnit

	ServerState

	NotAutomatic

	Unreadable
)

func (r Refusal) String() string {
	switch r {
	case NeedsContainer:
		return "needs a container"
	case OpaqueUnit:
		return "defined outside this repository"
	case ServerState:
		return "depends on state only CI has"
	case NotAutomatic:
		return "not triggered by a push"
	case Unreadable:
		return "could not be read"
	}

	return "unknown"
}

type Provider interface {
	Name() string

	Detect(dir string) (path string, found bool)

	Events() []string
}

var registry []Provider

func Register(p Provider) { registry = append(registry, p) }

func Providers() []Provider { return registry }

func Detect(dir string) (Provider, string, bool) {
	for _, p := range registry {
		if path, ok := p.Detect(dir); ok {
			return p, path, true
		}
	}

	return nil, "", false
}
