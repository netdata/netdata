#!/usr/bin/bash

convert_path() {
    local ARG="$1"
    ARG="${ARG//C:\\//c/}"
    ARG="${ARG//c:\\//c/}"
    ARG="${ARG//C:\///c/}"
    ARG="${ARG//c:\///c/}"

    echo "$ARG"
}

declare params=()
for x in "${@}"
do
  params+=("$(convert_path "${x}")")
done

"${params[@]}"
