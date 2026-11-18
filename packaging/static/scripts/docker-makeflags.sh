#!/usr/bin/env bash

if [ -z "${MAKEFLAGS}" ]; then
    echo "-e MAKEFLAGS='-j $(nproc)'"
    exit 0
fi

# GNU Make backslash-escapes whitespace and backslashes in MAKEFLAGS words.
# Match the complete FIFO word without splitting or decoding unrelated flags.
jobserver_pattern='(^|[[:space:]])--jobserver-auth=fifo:(([^\\[:space:]]|\\.)+)'
JOBSERVER_PATH=

if [[ ${MAKEFLAGS} =~ ${jobserver_pattern} ]]; then
    # Omitting -r deliberately decodes GNU Make's backslash escapes.
    # shellcheck disable=SC2162
    IFS= read JOBSERVER_PATH <<< "${BASH_REMATCH[2]}"
    MAKEFLAGS="${MAKEFLAGS/"${BASH_REMATCH[0]}"/${BASH_REMATCH[1]}--jobserver-auth=fifo:\/jobserver}"
fi

# The caller inserts this output into a /bin/sh command, so emit literal
# single quotes and escape embedded apostrophes for that shell to interpret.
printf -- "-e MAKEFLAGS='%s'" "${MAKEFLAGS//\'/\'\\\'\'}"
if [ -n "${JOBSERVER_PATH}" ]; then
    printf " -v '%s:/jobserver:rw'" "${JOBSERVER_PATH//\'/\'\\\'\'}"
fi
printf '\n'
