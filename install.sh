#!/bin/sh
set -eu

REPO=dangerous-person/zksnarks-cli
BIN=zksnarks
CURL_OPTS="-fsSL --connect-timeout 15 --retry 1 --speed-limit 1 --speed-time 30"
WGET_OPTS="-q -T 15"
SPIDER_OPTS="-S --spider -T 15"

esc=$(printf '\033')
grey= green= yellow= red= reset= ereset= hide= show= clear= live= tty=
fill='■'
dot='·'
pid=
tmp=
said=

program() {
	p=${TERM_PROGRAM:-}
	if { [ "$p" = tmux ] || [ "$p" = screen ]; } && [ "${__CFBundleIdentifier:-}" = com.apple.Terminal ]; then
		p=Apple_Terminal
	fi
	if [ "$p" = Apple_Terminal ] && [ -n "${KITTY_WINDOW_ID:-}${ALACRITTY_WINDOW_ID:-}${WEZTERM_EXECUTABLE:-}${GHOSTTY_RESOURCES_DIR:-}" ]; then
		p=
	fi
	printf '%s' "$p"
}

truecolor() {
	case "${TERM:-}" in
	screen*)
		if [ -z "${TMUX:-}" ]; then
			return 1
		fi
		;;
	esac
	case "$(printf '%s' "${COLORTERM:-}" | tr '[:upper:]' '[:lower:]')" in
	truecolor | 24bit) return 0 ;;
	esac
	if [ -n "${WT_SESSION:-}${KITTY_WINDOW_ID:-}${ALACRITTY_WINDOW_ID:-}${WEZTERM_EXECUTABLE:-}${GHOSTTY_RESOURCES_DIR:-}" ]; then
		return 0
	fi
	if [ "${TERMINAL_EMULATOR:-}" = JetBrains-JediTerm ] || [ "${LC_TERMINAL:-}" = iTerm2 ]; then
		return 0
	fi
	case "$(program)" in
	ghostty | iTerm.app | WezTerm | vscode | Hyper | rio | Tabby | WarpTerminal) return 0 ;;
	Apple_Terminal)
		major=$(uname -r 2>/dev/null | cut -d . -f 1)
		case "$major" in
		'' | *[!0-9]*) major=0 ;;
		esac
		if [ "$major" -ge 25 ]; then
			return 0
		fi
		;;
	esac
	case "${TERM:-}" in
	*kitty* | *ghostty* | *direct* | *alacritty*) return 0 ;;
	esac
	return 1
}

paint() {
	term=$(printenv TERM 2>/dev/null || true)
	if [ -t 1 ] && [ "$term" != dumb ]; then
		live=1
		hide="$esc[?25l"
		show="$esc[?25h"
		clear="$esc[K"
		if [ -z "${NO_COLOR:-}" ]; then
			if truecolor; then
				grey="$esc[38;2;139;139;148m"
				green="$esc[38;2;120;255;150m"
				yellow="$esc[38;2;244;211;94m"
			else
				grey="$esc[38;5;245m"
				green="$esc[38;5;120m"
				yellow="$esc[38;5;221m"
			fi
			reset="$esc[0m"
		fi
	fi
	if [ -t 2 ] && [ "$term" != dumb ] && [ -z "${NO_COLOR:-}" ]; then
		if truecolor; then
			red="$esc[38;2;239;107;107m"
		else
			red="$esc[38;5;203m"
		fi
		ereset="$esc[0m"
	fi
}

say() {
	printf '%s\n' "$*"
	if [ -n "$*" ]; then
		said=1
	fi
}

fail() {
	if [ -n "$said" ]; then
		printf '\n' >&2
	fi
	printf '%szksnarks install: %s%s\n' "$red" "$1" "$ereset" >&2
	shift
	for l in "$@"; do
		if [ -n "$l" ]; then
			printf '%s\n' "$l" >&2
		fi
	done
	printf '\n' >&2
	exit 1
}

mine() {
	if has curl; then
		printf 'curl -fsSL https://raw.githubusercontent.com/%s/main/install.sh | ZKSNARKS_INSTALL_DIR=~/.local/bin sh' "$REPO"
	else
		printf 'wget -qO- https://raw.githubusercontent.com/%s/main/install.sh | ZKSNARKS_INSTALL_DIR=~/.local/bin sh' "$REPO"
	fi
}

cleanup() {
	if [ -n "$pid" ]; then
		kill "$pid" 2>/dev/null || true
	fi
	if [ -n "$tmp" ]; then
		rm -rf "$tmp"
	fi
	if [ -n "$tty" ]; then
		stty "$tty" </dev/tty 2>/dev/null || true
	fi
	printf '%s' "$show" 2>/dev/null || true
}

has() {
	command -v "$1" >/dev/null 2>&1
}

fetch() {
	if has curl; then
		curl $CURL_OPTS -o "$2" "$1" 2>"$3"
	else
		wget $WGET_OPTS -O "$2" "$1" 2>"$3"
	fi
}

get() {
	if has curl; then
		exec curl $CURL_OPTS -o "$2" "$1" 2>"$3"
	fi
	exec wget $WGET_OPTS -O "$2" "$1" 2>"$3"
}

size() {
	if has curl; then
		curl -fsIL --connect-timeout 15 --max-time 30 "$1" 2>/dev/null | tr -d '\r' | awk '$1 ~ /^HTTP\// { ok = $2 ~ /^2/; n = 0 } ok && tolower($1) == "content-length:" { n = $2 } END { print n + 0 }'
	else
		wget $SPIDER_OPTS "$1" 2>&1 | tr -d '\r' | awk '$1 ~ /^HTTP\// { ok = $2 ~ /^2/; n = 0 } ok && tolower($1) == "content-length:" { n = $2 } END { print n + 0 }'
	fi
}

latest() {
	url=https://github.com/$REPO/releases/latest
	if has curl; then
		curl -fsSLI --connect-timeout 15 --max-time 30 -o /dev/null -w '%{url_effective}' "$url" 2>/dev/null
	else
		wget $SPIDER_OPTS "$url" 2>&1 | tr -d '\r' | awk 'tolower($1) == "location:" { l = $2 } END { print l }'
	fi
}

sha256() {
	if has sha256sum; then
		sha256sum "$1" | cut -d ' ' -f 1
	else
		shasum -a 256 "$1" | cut -d ' ' -f 1
	fi
}

columns() {
	c=$({ stty size </dev/tty; } 2>/dev/null | cut -d ' ' -f 2) || c=
	case "$c" in
	'' | *[!0-9]*) c=80 ;;
	esac
	printf '%s' "$c"
}

bytes() {
	n=$(ls -ln "$1" 2>/dev/null | awk '{ print $5 }')
	printf '%s' "${n:-0}"
}

draw() {
	n=0
	if [ "$2" -gt 0 ]; then
		pct=$(($1 * 100 / $2))
		if [ "$pct" -gt 100 ]; then
			pct=100
		fi
		n=$((pct * width / 100))
		right=$(printf '%3d%%' "$pct")
	else
		right=$(printf '%2dMB' $(($1 / 1048576)))
	fi
	on=
	off=
	i=0
	while [ "$i" -lt "$n" ]; do
		on=$on$fill
		i=$((i + 1))
	done
	while [ "$i" -lt "$width" ]; do
		off=$off$dot
		i=$((i + 1))
	done
	printf '\r%s%s%s %s%s' "$green" "$on" "$off" "$right" "$reset"
}

download() {
	if [ -z "$live" ]; then
		fetch "$1" "$2" "$3"
		return
	fi
	total=$(size "$1") || total=0
	case "$total" in
	'' | *[!0-9]*) total=0 ;;
	esac
	cols=$(columns)
	if [ "$cols" -ge 80 ]; then
		width=75
	elif [ "$cols" -ge 57 ]; then
		width=52
	else
		width=$((cols - 5))
	fi
	if [ "$width" -lt 10 ]; then
		width=10
	fi
	printf '%s' "$hide"
	tty=$({ stty -g </dev/tty; } 2>/dev/null) || tty=
	if [ -n "$tty" ]; then
		stty -echoctl </dev/tty 2>/dev/null || true
	fi
	get "$1" "$2" "$3" &
	pid=$!
	while kill -0 "$pid" 2>/dev/null; do
		draw "$(bytes "$2")" "$total"
		sleep 0.1 2>/dev/null || sleep 1
	done
	ok=1
	wait "$pid" || ok=
	pid=
	if [ -n "$tty" ]; then
		stty "$tty" </dev/tty 2>/dev/null || true
		tty=
	fi
	if [ -n "$ok" ]; then
		draw 1 1
		printf '%s\n' "$show"
	else
		printf '\r%s%s' "$clear" "$show"
		return 1
	fi
}

ask_sudo() {
	if sudo -n true 2>/dev/null; then
		return 0
	fi
	if ! { : </dev/tty; } 2>/dev/null; then
		fail "can't ask for your password here" "To install just for you, run:" "$(mine)"
	fi
	if [ "$1" = again ]; then
		say ""
	fi
	say "${grey}Enter your password to install into$reset $dir$grey.$reset"
	say "${grey}Nothing shows while you type.$reset"
	if ! sudo -v; then
		fail "can't install into $dir without sudo" "To install just for you, run:" "$(mine)"
	fi
	if [ "$1" = first ]; then
		say ""
	fi
}

reason() {
	r=$(grep -v 'previous errors' "$1" 2>/dev/null | grep -v '^ *$' | tail -n 1 | sed 's/^[0-9][0-9-]* [0-9][0-9:]* //') || r=
	printf '%s' "${r:-the connection failed or timed out}"
}

onpath() {
	case ":$PATH:" in
	*":$1:"* | *":$1/:"*) return 0 ;;
	esac
	return 1
}

remote() {
	if [ "$os" != linux ] || [ -z "${SSH_CONNECTION:-}" ] || [ -n "${PULSE_SERVER:-}" ]; then
		return 1
	fi
	case "$name" in
	*-no-audio) return 1 ;;
	esac
	if [ -n "${XDG_RUNTIME_DIR:-}" ] && [ -S "$XDG_RUNTIME_DIR/pulse/native" ]; then
		return 1
	fi
}

tip() {
	if [ -n "$live" ] && [ "$(program)" = Apple_Terminal ]; then
		say ""
		say "${yellow}For the best experience use Ghostty or iTerm2.$reset"
	fi
	if [ -n "$live" ] && remote; then
		say ""
		say "${yellow}Over SSH the app has no sound.$reset"
		say "${yellow}Install it on your own computer to hear it.$reset"
	fi
}

hints() {
	if onpath "$dir"; then
		active=$(command -v "$BIN" 2>/dev/null || true)
		if [ -n "$active" ] && ! [ "$active" -ef "$dir/$BIN" ]; then
			rm=rm
			if [ ! -w "$(dirname "$active")" ]; then
				rm="sudo rm"
			fi
			say ""
			say "${grey}Another $BIN runs first:$reset $active"
			say "${grey}Delete it with:$reset $rm $active"
		fi
		return
	fi
	line="export PATH=\"$dir:\$PATH\""
	case "${SHELL:-}" in
	*/zsh) rc='~/.zshrc' ;;
	*/bash)
		rc='~/.bashrc'
		if [ "$os" = darwin ]; then
			rc='~/.bash_profile'
		fi
		;;
	*/fish)
		rc='~/.config/fish/config.fish'
		line="fish_add_path \"$dir\""
		;;
	*) rc='your shell startup file' ;;
	esac
	say ""
	say "$dir ${grey}is not on your PATH yet.$reset"
	say "${grey}Add this line to$reset $rc$grey:$reset"
	say "$line"
	say "${grey}Then open a new terminal.$reset"
}

interrupted() {
	if [ -n "$live" ]; then
		printf '\r%s' "$clear"
	else
		printf '\n'
	fi
	exit 130
}

main() {
	paint
	trap cleanup EXIT
	trap interrupted INT
	trap 'exit 143' TERM
	trap 'exit 129' HUP
	trap 'exit 141' PIPE
	say ""

	os=$(uname -s)
	arch=$(uname -m)
	case "$os" in
	Darwin) os=darwin ;;
	Linux) os=linux ;;
	MINGW* | MSYS* | CYGWIN*)
		fail "on Windows, download the zip from the releases page:" \
			"https://github.com/$REPO/releases/latest"
		;;
	*) fail "there is no $BIN build for $os" ;;
	esac
	case "$arch" in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	armv7*)
		fail "there is no $BIN build for 32-bit ARM" \
			"On a Raspberry Pi, use the 64-bit Raspberry Pi OS."
		;;
	armv6*) fail "there is no $BIN build for 32-bit ARM" ;;
	*) fail "there is no $BIN build for $arch" ;;
	esac
	if [ "$os" = darwin ] && [ "$arch" = amd64 ] && [ "$(sysctl -n sysctl.proc_translated 2>/dev/null || true)" = 1 ]; then
		arch=arm64
	fi

	name=$BIN-$os-$arch
	if [ "$os" = linux ]; then
		loader=/lib64/ld-linux-x86-64.so.2
		if [ "$arch" = arm64 ]; then
			loader=/lib/ld-linux-aarch64.so.1
		fi
		case "$(printf '%s' "${ZKSNARKS_NO_AUDIO:-}" | tr '[:upper:]' '[:lower:]')" in
		1 | true | yes) name=$name-no-audio ;;
		*)
			if [ ! -e "$loader" ]; then
				name=$name-no-audio
			fi
			;;
		esac
	fi

	if ! has curl && ! has wget; then
		fail "needs curl or wget"
	fi
	if ! has curl && wget --version >/dev/null 2>&1; then
		WGET_OPTS="-nv --tries=3 -T 15"
		SPIDER_OPTS="-S --spider -T 15 --tries=3"
	fi
	if ! has sha256sum && ! has shasum; then
		fail "needs sha256sum or shasum to check the download"
	fi

	if [ -n "${ZKSNARKS_DOWNLOAD_URL:-}" ]; then
		base=$ZKSNARKS_DOWNLOAD_URL
		version=${ZKSNARKS_VERSION:-}
	elif [ -n "${ZKSNARKS_VERSION:-}" ]; then
		version=${ZKSNARKS_VERSION#v}
		base=https://github.com/$REPO/releases/download/v$version
	else
		tag=$(latest | sed 's|.*/tag/||') || tag=
		case "$tag" in
		v[0-9]*) ;;
		*) fail "can't find the latest release" "https://github.com/$REPO/releases" ;;
		esac
		case "$tag" in
		*[!A-Za-z0-9._-]*) fail "can't read the latest release tag" ;;
		esac
		version=${tag#v}
		base=https://github.com/$REPO/releases/download/$tag
	fi
	version=${version#v}

	existing=$(command -v "$BIN" 2>/dev/null || true)
	if [ -n "${ZKSNARKS_INSTALL_DIR:-}" ]; then
		dir=$ZKSNARKS_INSTALL_DIR
		case "$dir" in
		"~" | "~/"*)
			if [ -z "${HOME:-}" ]; then
				fail "HOME is not set, give ZKSNARKS_INSTALL_DIR as a full path"
			fi
			;;
		esac
		case "$dir" in
		"~") dir=$HOME ;;
		"~/"*) dir=$HOME/${dir#"~/"} ;;
		esac
		case "$dir" in
		/*) ;;
		*) dir=$(pwd)/$dir ;;
		esac
		while [ "$dir" != / ] && [ "${dir%/}" != "$dir" ]; do
			dir=${dir%/}
		done
	else
		dir=/usr/local/bin
		case "$existing" in
		/*)
			if [ -f "$existing" ]; then
				dir=$(dirname "$existing")
			fi
			;;
		esac
	fi

	if [ -d "$dir/$BIN" ]; then
		fail "$dir/$BIN is a folder, not the program"
	fi
	old=
	if [ -x "$dir/$BIN" ]; then
		old=$("$dir/$BIN" version 2>/dev/null | cut -d ' ' -f 2) || old=
	fi
	if [ -n "$old" ] && [ "$old" = "$version" ]; then
		say "$BIN $version ${grey}is already installed.$reset"
		tip
		hints
		"$dir/$BIN" help 2>/dev/null || say ""
		return
	fi

	sudo=
	p=$dir
	while [ ! -d "$p" ]; do
		p=$(dirname "$p")
	done
	if [ ! -w "$p" ]; then
		if ! has sudo; then
			fail "can't write to $dir" "To install just for you, run:" "$(mine)"
		fi
		sudo=sudo
	fi

	if [ -n "$sudo" ]; then
		ask_sudo first
	fi
	if [ -n "$old" ] && [ -n "$version" ]; then
		say "${grey}Updating$reset $BIN $grey$old to$reset $version"
	elif [ -n "$old" ]; then
		say "${grey}Updating$reset $BIN $grey$old$reset"
	else
		say "${grey}Installing$reset $BIN${version:+ $version}"
	fi

	tmp=$(mktemp -d)
	if ! download "$base/$name.tar.gz" "$tmp/$name.tar.gz" "$tmp/err"; then
		fail "can't download the release" "$(reason "$tmp/err")" "$base/$name.tar.gz"
	fi
	if ! fetch "$base/checksums.txt" "$tmp/checksums.txt" "$tmp/err"; then
		fail "can't download the checksums" "$(reason "$tmp/err")" "$base/checksums.txt"
	fi
	want=$(awk -v f="$name.tar.gz" '{ sub(/\r$/, "") } $2 == f || $2 == "*" f { print $1; exit }' "$tmp/checksums.txt")
	if [ -z "$want" ]; then
		fail "no checksum for $name.tar.gz"
	fi
	if [ "$want" != "$(sha256 "$tmp/$name.tar.gz")" ]; then
		fail "the download is damaged, please try again"
	fi
	if ! tar -xzf "$tmp/$name.tar.gz" -C "$tmp" "$BIN" 2>"$tmp/err"; then
		fail "can't unpack $name.tar.gz" "$(reason "$tmp/err")"
	fi
	chmod 755 "$tmp/$BIN"
	if [ "$os" = darwin ]; then
		xattr -d com.apple.quarantine "$tmp/$BIN" 2>/dev/null || true
	fi

	if [ -n "$sudo" ]; then
		ask_sudo again
	fi
	$sudo mkdir -p "$dir" || fail "can't create $dir"
	$sudo rm -f "$dir/.$BIN.new"
	$sudo cp "$tmp/$BIN" "$dir/.$BIN.new" || fail "can't write to $dir"
	$sudo chmod 755 "$dir/.$BIN.new" || fail "can't write to $dir"
	if ! "$dir/.$BIN.new" version >/dev/null 2>&1; then
		$sudo rm -f "$dir/.$BIN.new"
		fail "the new $BIN does not start, nothing was changed"
	fi
	$sudo mv -f "$dir/.$BIN.new" "$dir/$BIN" || fail "can't write to $dir"
	tip

	hints
	"$dir/$BIN" help
}

main "$@"
