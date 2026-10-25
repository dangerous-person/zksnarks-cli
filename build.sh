#!/bin/sh
set -eu

trap 'status=$?; if [ "$status" -ne 0 ]; then echo "build.sh: FAILED with exit code $status" >&2; fi' EXIT

cd -- "$(dirname -- "$0")"

if [ -n "${1:-}" ]; then
	VERSION=$1
else
	VERSION=$(git describe --tags --always --dirty)
fi

GO_VERSION=$(go version)
DIST=dist
BUILT=

echo "zksnarks $VERSION with $GO_VERSION"

rm -rf "$DIST"
mkdir -p "$DIST"

build() {
	os=$1
	arch=$2
	tags=${3:-}
	name=zksnarks-$os-$arch
	if [ "$tags" = noaudio ]; then
		name=$name-no-audio
	fi
	if [ "$os" = windows ]; then
		name=$name.exe
	fi
	echo "building $name"
	CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -tags "$tags" -ldflags "-s -w -X main.version=$VERSION" -o "$DIST/$name" ./cmd/zksnarks
	BUILT="$BUILT $name"
}

build darwin arm64
build darwin amd64
build linux amd64
build linux amd64 noaudio
build linux arm64
build linux arm64 noaudio
build windows amd64

if command -v sha256sum >/dev/null 2>&1; then
	SHA256="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
	SHA256="shasum -a 256"
else
	echo "build.sh: neither sha256sum nor shasum is installed" >&2
	exit 1
fi

echo "writing $DIST/checksums.txt with $SHA256"
(cd "$DIST" && $SHA256 $BUILT) >"$DIST/checksums.txt"

echo
printf '%-34s %12s %9s\n' FILE BYTES MB
for name in $BUILT checksums.txt; do
	bytes=$(wc -c <"$DIST/$name")
	bytes=$((bytes))
	mb=$(awk -v b="$bytes" 'BEGIN { printf "%.2f", b / 1000000 }')
	printf '%-34s %12s %9s\n' "$name" "$bytes" "$mb"
done
