#!/bin/sh
set -eu

trap 'status=$?; if [ "$status" -ne 0 ]; then echo "build.sh: FAILED with exit code $status" >&2; fi' EXIT

cd -- "$(dirname -- "$0")"

refuse() {
	echo "build.sh: $*" >&2
	trap - EXIT
	exit 1
}

DOCS="README.md LICENSE THIRD_PARTY_NOTICES.md"

if [ -n "${1:-}" ]; then
	VERSION=$1
	if ! printf '%s\n' "$VERSION" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
		refuse "a release version looks like v1.0.0"
	fi
	for f in $DOCS; do
		if [ ! -f "$f" ]; then
			refuse "$f is missing, every release archive ships it"
		fi
	done
	changes=$(git status --porcelain) || refuse "can't read the git status"
	if [ -n "$changes" ]; then
		refuse "$VERSION would be built from uncommitted changes, commit them first"
	fi
	tagged=$(git rev-parse -q --verify "refs/tags/$VERSION^{commit}" 2>/dev/null) || tagged=
	if [ -z "$tagged" ]; then
		refuse "there is no tag $VERSION yet, make it first: git tag -a $VERSION -m $VERSION"
	fi
	if [ "$tagged" != "$(git rev-parse HEAD)" ]; then
		refuse "tag $VERSION points at another commit, check out that commit or move the tag"
	fi
else
	VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo dev)
fi

for tool in go zip gzip tar; do
	if ! command -v "$tool" >/dev/null 2>&1; then
		refuse "$tool is not installed"
	fi
done

if command -v sha256sum >/dev/null 2>&1; then
	SHA256="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
	SHA256="shasum -a 256"
else
	refuse "neither sha256sum nor shasum is installed"
fi

STAMP=$(TZ=UTC git log -1 --format=%cd --date=format-local:%Y%m%d%H%M.%S 2>/dev/null || echo 202601010000.00)

GO_VERSION=$(go version)
DIST=dist
BIN=$DIST/bin
BUILT=

echo "zksnarks $VERSION with $GO_VERSION"

rm -rf "$DIST"
mkdir -p "$BIN"

if tar --version 2>/dev/null | grep -q bsdtar; then
	TAR_FLAGS="--no-xattrs --no-mac-metadata --no-acls --uid 0 --gid 0 --uname root --gname root"
else
	TAR_FLAGS="--owner=0 --group=0 --numeric-owner"
fi

HAVE=
for f in $DOCS; do
	if [ -f "$f" ]; then
		HAVE="$HAVE $f"
	fi
done

build() {
	os=$1
	arch=$2
	tags=${3:-}
	name=zksnarks-$os-$arch
	if [ "$tags" = noaudio ]; then
		name=$name-no-audio
	fi
	exe=zksnarks
	if [ "$os" = windows ]; then
		exe=zksnarks.exe
	fi
	echo "building $name"
	mkdir -p "$BIN/$name"
	CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -tags "$tags" -ldflags "-s -w -X main.version=$VERSION" -o "$BIN/$name/$exe" ./cmd/zksnarks
	chmod 755 "$BIN/$name/$exe"
	for f in $HAVE; do
		cp "$f" "$BIN/$name/"
		chmod 644 "$BIN/$name/$f"
	done
	(cd "$BIN/$name" && TZ=UTC touch -t "$STAMP" $exe $HAVE)
	if [ "$os" = windows ]; then
		archive=$name.zip
		(cd "$BIN/$name" && TZ=UTC zip -q -X "../../$archive" $exe $HAVE)
	else
		archive=$name.tar.gz
		COPYFILE_DISABLE=1 tar $TAR_FLAGS -cf "$DIST/$name.tar" -C "$BIN/$name" $exe $HAVE
		gzip -n -9 "$DIST/$name.tar"
	fi
	BUILT="$BUILT $archive"
}

build darwin arm64
build darwin amd64
build linux amd64
build linux amd64 noaudio
build linux arm64
build linux arm64 noaudio
build windows amd64

echo "writing $DIST/checksums.txt with $SHA256"
(cd "$DIST" && $SHA256 $BUILT) >"$DIST/checksums.txt"

echo
printf '%-40s %12s %9s\n' FILE BYTES MB
for name in $BUILT checksums.txt; do
	bytes=$(wc -c <"$DIST/$name")
	bytes=$((bytes))
	mb=$(awk -v b="$bytes" 'BEGIN { printf "%.2f", b / 1000000 }')
	printf '%-40s %12s %9s\n' "$name" "$bytes" "$mb"
done
echo
if [ -n "${1:-}" ]; then
	echo "release:"
	echo "  git push origin main $VERSION"
	echo "  gh release create $VERSION --verify-tag $DIST/*.tar.gz $DIST/*.zip $DIST/checksums.txt"
fi
echo "binaries for local tests: $BIN"
