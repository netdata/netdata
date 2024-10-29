#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later

set -e

function txt_to_rtf() {
    INPUT="$1"
    OUTPUT="$2"

    echo "{\rtf1\ansi\deff0 {\fonttbl {\f0 Times New Roman;}}" > "$OUTPUT"
    echo "\paperh15840 \paperw12240" >> "$OUTPUT"
    echo "\margl720 \margr720 \margt720 \margb720" >> "$OUTPUT"
    echo "\f0\fs24" >> "$OUTPUT"

    sed s/\$/'\\line'/ "$INPUT" | sed s/\\f/'\\page'/ >> "$OUTPUT"
    echo "}" >> "$OUTPUT"
}

if [ ! -f "gpl-3.0.txt" ]; then
    curl -o gpl-3.0.txt "https://www.gnu.org/licenses/gpl-3.0.txt"
fi

if [ ! -f "cloud.txt" ]; then
    curl -o cloud.txt "https://raw.githubusercontent.com/netdata/netdata/master/src/web/gui/v2/LICENSE.md"
fi

if [ -f "gpl-3.0.txt" ] ; then
    txt_to_rtf "gpl-3.0.txt" "gpl-3.0.rtf"
fi

if [ -f "cloud.txt" ] ; then
    txt_to_rtf "cloud.txt" "cloud.rtf"
fi
