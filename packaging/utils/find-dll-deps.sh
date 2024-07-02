#!/usr/bin/env bash

if [ "$#" -lt 1 ]; then
    echo "Usage: $0 <command1> <command2> ... <commandN>"
    exit 1
fi

results=()

for arg in "$@"; do
    while IFS= read -r line; do
      results+=("$line")
    done < <(ldd "$arg" | grep /usr/bin | awk '{ print $3 }')
done

printf "%s\n" "${results[@]}" | sort | uniq
