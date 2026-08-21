package git

var (
	cmdRoot        = []string{"rev-parse", "--show-toplevel"}
	cmdGitDir      = []string{"rev-parse", "--git-dir"}
	cmdCommonDir   = []string{"rev-parse", "--git-common-dir"}
	cmdHooksPath   = []string{"rev-parse", "--git-path", "hooks"}
	cmdHooksConfig = []string{"config", "--get", "core.hooksPath"}
	cmdBranch      = []string{"branch", "--show-current"}
	cmdHead        = []string{"rev-parse", "HEAD"}
	cmdOriginURL   = []string{"remote", "get-url", "origin"}
	cmdOriginHead  = []string{"symbolic-ref", "refs/remotes/origin/HEAD"}
	cmdUpstream    = []string{"rev-parse", "--abbrev-ref", "@{upstream}"}

	cmdStatus  = []string{"--no-optional-locks", "status", "--porcelain", "-z", "--untracked-files=all"}
	cmdIndex   = []string{"ls-files", "-s", "-z"}
	cmdVersion = []string{"version"}
)

func cmdVerify(rev string) []string { return []string{"rev-parse", "--verify", rev} }

func cmdMergeBase(a, b string) []string { return []string{"merge-base", a, b} }

func cmdDiff(base string) []string {
	return []string{"diff", "--name-only", "-z", "--no-ext-diff", "--no-renames", base}
}
