#!/usr/bin/env sh

base="$(dirname "${0}")"

os="${1}"

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

echo >&2 "generating dashboard_info.js for '${os}'..."

cat "${base}/web/dashboard_info_header.js" || exit 1

for x in menu submenu context
do
	cat "${base}/web/dashboard_info_${x}_pre.js" || exit 1

	test -f "${base}/web/dashboard_info_${x}_${os}.js" && \
		cat "${base}/web/dashboard_info_${x}_${os}.js"

	cat "${base}/web/dashboard_info_${x}_post.js" || exit 1
done

cat "${base}/web/dashboard_info_footer.js" || exit 1
