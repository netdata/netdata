#!/usr/bin/env bash

if [ -z "${MAKEFLAGS}" ]; then
    echo "-e MAKEFLAGS='-j $(nproc)'"
    exit 0
fi

IFS=' ' read -ra flags <<< "${MAKEFLAGS}"
jobserver_index=-1

for i in "${!flags[@]}"; do
    if [[ ${flags[i]} == --jobserver-auth=* ]]; then
        jobserver_index=$i
        break
    fi
done

if [ "${jobserver_index}" -gt -1 ]; then
    JOBSERVER_PATH="$(echo "${flags[${jobserver_index}]}" | cut -d ':' -f 2)"
    unset "flags[${jobserver_index}]"
    flags=("${flags[@]}" --jobserver-auth=fifo:/jobserver)
fi

MAKEFLAGS="${flags[*]}"

echo "-e MAKEFLAGS='${MAKEFLAGS}' ${JOBSERVER_PATH:+-v "${JOBSERVER_PATH}:/jobserver:rw"}"
