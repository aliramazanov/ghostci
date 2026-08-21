package git

import "strings"

type Info struct {
	Root       string
	Branch     string
	Ref        string
	Repository string
	Head       string
	IsRepo     bool

	DefaultBranch string
}

func Discover(dir string) Info {
	var info Info

	root, err := Root(dir)
	if err != nil {
		return info
	}

	info.Root, info.IsRepo = root, true

	if branch, err := Run(dir, cmdBranch...); err == nil && branch != "" {
		info.Branch = branch
		info.Ref = "refs/heads/" + branch
	}

	if url, err := Run(dir, cmdOriginURL...); err == nil {
		info.Repository = parseRemote(url)
	}

	if head, err := Run(dir, cmdHead...); err == nil {
		info.Head = head
	}

	if ref, err := Run(dir, cmdOriginHead...); err == nil {
		info.DefaultBranch = strings.TrimPrefix(ref, "refs/remotes/origin/")
	}

	return info
}

func parseRemote(url string) string {
	url = strings.TrimSuffix(strings.TrimSpace(url), ".git")

	if i := strings.Index(url, "://"); i >= 0 {
		url = url[i+3:]
		if at := strings.Index(url, "@"); at >= 0 {
			url = url[at+1:]
		}
	} else if at := strings.Index(url, "@"); at >= 0 {
		url = url[at+1:]
	}

	url = strings.Replace(url, ":", "/", 1)

	parts := strings.Split(url, "/")
	if len(parts) < 3 {
		return ""
	}

	return strings.Join(parts[len(parts)-2:], "/")
}
