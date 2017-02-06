#!/usr/bin/env sh

base="$(dirname "${0}")"

target="${1}"
os="${2}"

case "${os}" in
	*linux*|*Linux*|*LINUX*)
		os="linux"
		;;

	*freebsd*|*FreeBSD*|*FREEBSD*)
		os="freebsd"
		;;

	*darwin*|*Darwin*|*DARWIN*|*macos*|*MacOS*|*MACOS*)
		os="macos"
		;;

	*)
		echo >&2 "Unknown OS '${os}'. One of 'linux', 'freebsd' or 'macos' is needed. Cannot continue."
		exit 1
esac

echo >&2 "generating '${target}' for '${os}'..."

if [ -f "${base}/conf.d/${os}/${target}" ]
	then
	echo >&2 "Found '${target}' for '${os}'."
	cat "${base}/conf.d/${os}/${target}"
else
	echo >&2 "Not found '${target}' for '${os}'. Using an empty one."
fi
