#!/usr/bin/bash

export MSYSTEM=MSYS
export PATH="/usr/bin:${PATH}"

source /etc/profile

convert_path() {
    local ARG="$1"
    ARG="${ARG//C:\\msys64\///}"
    ARG="${ARG//c:\\msys64\///}"
    ARG="${ARG//C:\/msys64\///}"
    ARG="${ARG//c:\/msys64\///}"

    echo "$ARG"
}

declare params=()
for x in "${@}"
do
	#echo "Parameter: '${x}'"
	y="$(convert_path "${x}")"
	#echo "Coverted: '${y}'"
	params+=("${y}")
done

# (
# 	printf "Running: "
# 	printf " %q" "${params[@]}"
# 	printf "\n"
# )

"${params[@]}"

