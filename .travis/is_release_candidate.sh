#!/bin/bash

DESCR=$(git describe)

if [[ ${DESCR} =~ -rc* ]]; then
	echo "This is a release candidate ${DESCR}"
	exit 0
fi

echo "${DESCR} is not a release candidate"
exit 1
