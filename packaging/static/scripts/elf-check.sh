#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later

set -e

file="${1}"
machine="${2}"
class="${3}"

elf_info=$(LC_ALL=C readelf -h "${file}")
file_machine=$(echo "${elf_info}" | grep 'Machine:' | awk -F: '{print $2}' | xargs)
file_class=$(echo "${elf_info}" | grep 'Class:' | awk -F: '{print $2}' | xargs)

if [ "${file_class}" != "${class}" ]; then
    echo "ERROR: ${file} has wrong ELF class (${file_class} instead of ${class})"
    echo "This indicates a 32-bit/64-bit mismatch!"
    exit 1
fi

if ! echo "${file_machine}" | grep -q "${machine}"; then
    echo "ERROR: ${file} was built for the wrong architecture (${file_machine} does not contain ${machine})"
    exit 1
fi
