#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Synthetic values; no helper or JSON encoder needed for fixed labels.
set -euo pipefail

snapshot() {
    printf 'temperature:%s|gauge|unit:Celsius\n' 21.5
    printf 'queue.depth:%s|gauge|#queue:mail|unit:jobs\n' 17
    printf 'requests_total:%s|counter|#service:api|unit:requests|title:API requests\n' 12345
}

case ${1:-} in
    collect) snapshot ;;
    serve)
        printf '%s\n' '{"version":"v1","ready":true}'
        request_pattern='^\{"id":"([1-9][0-9]*)","method":"collect"\}$'
        while IFS= read -r request; do
            if [[ ! $request =~ $request_pattern ]]; then
                printf '%s\n' 'expected a collection request' >&2
                exit 2
            fi
            request_id=${BASH_REMATCH[1]}
            snapshot
            printf '# EOF %s\n' "$request_id"
        done
        ;;
    *) printf '%s\n' 'expected collect or serve' >&2; exit 2 ;;
esac
