#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later

if ! hash curl 2>/dev/null
then
	1>&2 echo "'curl' not found on system. Please install 'curl'."
	exit 1
fi

# set the host to connect to
if [ ! -z "$1" ]
then
	host="$1"
else
	host="http://127.0.0.1:19999"
fi
echo "using netdata server at: $host"

# shellcheck disable=SC2207 disable=SC1117
charts=($(curl "$host/netdata.conf" 2>/dev/null | grep "^\[" | cut -d '[' -f 2 | cut -d ']' -f 1 | grep -v ^global$ | grep -v "^plugin" | sort -u))
if [ "${#charts[@]}" -eq 0 ]
then
	echo "Cannot download charts from server: $host"
	exit 1
fi

update_every="$(curl "$host/netdata.conf" 2>/dev/null | grep "update every = " | head -n 1 | cut -d '=' -f 2)"
[ $(( update_every + 1 - 1)) -eq 0 ] && update_every=1

entries="$(curl "$host/netdata.conf" 2>/dev/null | grep "history = " | head -n 1 | cut -d '=' -f 2)"
[ $(( entries + 1 - 1)) -eq 0 ] && entries=3600

# to compare equal things, set the entries to 3600 max
[ $entries -gt 3600 ] && entries=3600

if [ $entries -ne 3600 ]
then
	echo >&2 "You are running a test for a history of $entries entries."
fi

modes=("average" "max")
formats=("jsonp" "json" "ssv" "csv" "datatable" "datasource" "tsv" "ssvcomma" "html" "array")
options="flip|jsonwrap"

now=$(date +%s)
first=$((now - (entries * update_every)))
duration=$((now - first))

file="$(mktemp /tmp/netdata-stress-XXXXXXXX)"
cleanup() {
	echo "cleanup"
	[ -f "$file" ] && rm "$file"
}
trap cleanup EXIT

while true
do
	echo "curl --compressed --keepalive-time 120 --header \"Connection: keep-alive\" \\" >"$file"
	# shellcheck disable=SC2034
	for x in {1..100}
	do
		dt=$((RANDOM * duration / 32767))
		st=$((RANDOM * duration / 32767))
		et=$(( st + dt ))
		[ $et -gt "$now" ] && st=$(( now - dt ))

		points=$((RANDOM * 2000 / 32767 + 2))
		st=$((first + st))
		et=$((first + et))

		mode=$((RANDOM * ${#modes[@]} / 32767))
		mode="${modes[$mode]}"

		chart=$((RANDOM * ${#charts[@]} / 32767))
		chart="${charts[$chart]}"

		format=$((RANDOM * ${#formats[@]} / 32767))
		format="${formats[$format]}"

		echo "--url \"$host/api/v1/data?chart=$chart&mode=$mode&format=$format&options=$options&after=$st&before=$et&points=$points\" \\"
	done >>"$file"
	bash "$file" >/dev/null
done
