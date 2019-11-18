#!/usr/bin/env bash

# Prevent travis from timing out after 10 minutes of no output
tick() {
	(while true; do sleep 300; echo; done) &
	local PID=$!
	disown

	"$@"
	local RET=$?

	kill $PID
	return $RET
}

retry() {
	local tries=$1
	shift

	local i=0
	while [ "$i" -lt "$tries" ]; do
		"$@" && return 0
		sleep $((2**((i++))))
	done

	return 1
}
