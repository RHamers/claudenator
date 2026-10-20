#!/usr/bin/env bash
# Builds and installs the claudenator binary via `go install`, then checks
# whether the Go bin directory is on PATH so it's runnable from anywhere.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

echo "Installing claudenator via 'go install .'..."
go install .

gobin="$(go env GOBIN)"
if [[ -z "$gobin" ]]; then
	gobin="$(go env GOPATH)/bin"
fi

echo "Installed to: $gobin/claudenator"

case ":$PATH:" in
*":$gobin:"*)
	echo "$gobin is already on your PATH — run 'claudenator' from any directory."
	;;
*)
	echo
	echo "$gobin is not on your PATH yet. Add this to your shell rc file (e.g. ~/.bashrc):"
	echo
	echo "  export PATH=\"$gobin:\$PATH\""
	echo
	echo "Then restart your shell (or 'source' the rc file)."
	;;
esac
