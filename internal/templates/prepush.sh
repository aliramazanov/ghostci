#!/bin/sh
# ghostci:managed
# Installed by ghostci. Remove with: ghostci uninstall-hook
#
# Git writes the refs being pushed to this hook's stdin. Every exit path below
# drains it, because exiting with input unread makes git fail with SIGPIPE.

if [ "$GHOSTCI_VERBOSE" = "1" ]; then
	set -x
fi

if [ -n "$GHOSTCI_SKIP" ]; then
	cat >/dev/null
	exit 0
fi

# Prefer an explicit override, then PATH, then the binary that installed this
# hook. Without the last fallback a ghostci outside PATH would make the hook
# skip silently, and the push would look checked when nothing ran.
if [ -n "$GHOSTCI_BIN" ] && [ -x "$GHOSTCI_BIN" ]; then
	exec "$GHOSTCI_BIN" --hook "$@"
elif command -v ghostci >/dev/null 2>&1; then
	exec ghostci --hook "$@"
elif [ -x {{.Binary}} ]; then
	exec {{.Binary}} --hook "$@"
fi

cat >/dev/null
echo "ghostci: not found on PATH and "{{.Binary}}" is gone; NO CHECKS RAN" >&2
echo "ghostci: set GHOSTCI_BIN, or reinstall the hook with: ghostci install-hook" >&2
exit 0
