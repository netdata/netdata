#!/usr/bin/env bash

set -e

LAST_MODIFICATION="$(git log -1 --pretty="format:%at" CHANGELOG.md)"
CURRENT_TIME="$(date +"%s")"
TWO_DAYS_IN_SECONDS=172800

DIFF=$((CURRENT_TIME - LAST_MODIFICATION))

echo "Checking CHANGELOG.md last modification time on GIT.."
echo "CHANGELOG.md timestamp: ${LAST_MODIFICATION}"
echo "Current timestamp: ${CURRENT_TIME}"
echo "Diff: ${DIFF}"

if [ ${DIFF} -gt ${TWO_DAYS_IN_SECONDS} ]; then
	echo "CHANGELOG.md is more than two days old!"
	post_message "TRAVIS_MESSAGE" "Hi <!here>, CHANGELOG.md was found more than two days old (Diff: ${DIFF} seconds)" "${NOTIF_CHANNEL}"
else
	echo "CHANGELOG.md is less than two days old, fine"
fi
