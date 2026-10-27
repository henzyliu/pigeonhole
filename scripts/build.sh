#!/bin/sh
# Builds Pigeonhole with its commit, branch and build date stamped in (shown
# under Settings → About).
#
#   scripts/build.sh            the Mac app (build/bin/Pigeonhole.app)
#   scripts/build.sh windows    a Windows exe (build/bin/Pigeonhole.exe),
#                               cross-compiled with MinGW (brew install mingw-w64)
#
# The GIF picker needs a GIPHY API key, taken from $GIPHY_API_KEY or the
# file .giphy-key (kept out of git). Without one, the app has no GIFs.
set -eu
cd "$(dirname "$0")/.."

commit=$(git rev-parse --short HEAD)
if [ -n "$(git status --porcelain)" ]; then
	commit="$commit+changes"
fi
giphy_key=${GIPHY_API_KEY:-}
if [ -z "$giphy_key" ] && [ -f .giphy-key ]; then
	giphy_key=$(tr -d '[:space:]' < .giphy-key)
fi
ldflags="-X main.commit=$commit -X main.branch=$(git rev-parse --abbrev-ref HEAD) -X main.buildDate=$(date -u +%Y-%m-%d) -X main.giphyKey=$giphy_key"
wails=$(command -v wails || echo "$HOME/go/bin/wails")

case "${1:-mac}" in
mac)
	"$wails" build -ldflags "$ldflags"
	;;
windows)
	CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ \
		"$wails" build -platform windows/amd64 -o Pigeonhole.exe -ldflags "$ldflags"
	;;
*)
	echo "usage: $0 [mac|windows]" >&2
	exit 2
	;;
esac
# wails build -clean deletes this placeholder, which keeps the folder in git.
touch frontend/dist/.gitkeep
