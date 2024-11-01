#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later

set -e

function txt_to_rtf() {
    INPUT="$1"
    OUTPUT="$2"

    echo '{\rtf1\ansi\deff0 {\fonttbl {\f0 Times New Roman;}}' > "$OUTPUT"
    echo '\paperh15840 \paperw12240' >> "$OUTPUT"
    echo '\margl720 \margr720 \margt720 \margb720' >> "$OUTPUT"
    echo '\f0\fs24' >> "$OUTPUT"

    sed s/\$/'\\line'/ "$INPUT" | sed s/\\f/'\\page'/ >> "$OUTPUT"
    echo '}' >> "$OUTPUT"
}

function check_and_get_file() {
    if [ ! -f "$1" ]; then
        curl -o "tmp.txt" "$2"
        txt_to_rtf "tmp.txt" "$1"
        rm  "tmp.txt"
    fi
}

check_and_get_file "gpl-3.0.rtf" "https://www.gnu.org/licenses/gpl-3.0.txt"
check_and_get_file "ncul1.rtf" "https://app.netdata.cloud/LICENSE.txt"

