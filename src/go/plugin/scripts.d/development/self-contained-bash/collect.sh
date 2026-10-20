#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Static values need only Bash and cat; dynamic strings require a JSON encoder.
set -eu
mode=oneshot
if [[ $1 == --persistent ]]; then mode=persistent; shift; fi
case "$1" in
    describe)
        printf 'version: v1\nmode: %s\n' "$mode"
        cat <<'DESCRIPTION'
metrics:
  - {name: depth, type: gauge, unit: jobs}
checks:
  - {id: ready, title: Queue readiness}
charts: |
  version: v1
  context_namespace: selfcontained_bash
  groups:
    - family: Queue
      metrics: [depth]
      charts:
        - id: depth
          title: Queue depth
          context: depth
          units: jobs
          instances:
            by_labels: [worker]
          dimensions: [{selector: depth, name: depth}]
DESCRIPTION
        ;;
    collect)
        printf '%s\n' '{"version":"v1","metrics":[{"name":"depth","value":17,"labels":{"worker":"main"}}],"checks":[{"id":"ready","state":"ok"}]}'
        ;;
    serve)
        printf '%s\n' '{"version":"v1","ready":true}'
        pattern='^\{"id":"([1-9][0-9]*)","method":"collect"\}$'
        while IFS= read -r request; do
            [[ $request =~ $pattern ]] || exit 2
            printf '{"id":"%s","result":{"version":"v1","metrics":[{"name":"depth","value":17,"labels":{"worker":"main"}}],"checks":[{"id":"ready","state":"ok"}]}}\n' "${BASH_REMATCH[1]}"
        done
        ;;
    *) printf 'Unsupported operation\n' >&2; exit 2 ;;
esac
