#!/usr/bin/env bash
set -euo pipefail

CB_HOST="${CB_HOST:-couchbase}"
CB_ADMIN="${CB_ADMIN:-Administrator}"
CB_PASS="${CB_PASS:-password}"

cb_ready=0
for i in $(seq 1 60); do
  code="$(curl -s -o /dev/null -w '%{http_code}' "http://${CB_HOST}:8091/pools" || true)"
  if [ "$code" = "200" ] || [ "$code" = "401" ]; then
    cb_ready=1
    break
  fi
  sleep 2
done

if [ "$cb_ready" -ne 1 ]; then
  echo "Couchbase API not ready after 120s" >&2
  exit 1
fi

cb_initialized_code="$(curl -s -o /dev/null -w '%{http_code}' "http://${CB_HOST}:8091/pools/default" -u "${CB_ADMIN}:${CB_PASS}" || true)"
if [ "$cb_initialized_code" != "200" ]; then
  /opt/couchbase/bin/couchbase-cli cluster-init -c "${CB_HOST}:8091" \
    --cluster-username "${CB_ADMIN}" \
    --cluster-password "${CB_PASS}" \
    --services data,index,query \
    --cluster-ramsize 256 \
    --cluster-index-ramsize 256 || true

  /opt/couchbase/bin/couchbase-cli bucket-create -c "${CB_HOST}:8091" \
    -u "${CB_ADMIN}" -p "${CB_PASS}" \
    --bucket default \
    --bucket-type couchbase \
    --bucket-ramsize 256 || true
fi

query_ready=0
for i in $(seq 1 60); do
  if curl -fsS "http://${CB_HOST}:8093/query/service" -u "${CB_ADMIN}:${CB_PASS}" -d "statement=SELECT 1" > /dev/null; then
    query_ready=1
    break
  fi
  sleep 2
done

if [ "$query_ready" -ne 1 ]; then
  echo "Couchbase query service not ready after 120s" >&2
  exit 1
fi

run_query() {
  local stmt="$1"
  local i
  for i in $(seq 1 30); do
    if curl -fsS -u "${CB_ADMIN}:${CB_PASS}" "http://${CB_HOST}:8093/query/service" \
      --data-urlencode "statement=${stmt}" > /dev/null; then
      return 0
    fi
    sleep 2
  done
  return 1
}

run_query "CREATE PRIMARY INDEX ON \`default\`" || true

run_query "INSERT INTO \`default\` (KEY, VALUE) VALUES (\"k1\", {\"type\":\"t\",\"v\":1})"
run_query "INSERT INTO \`default\` (KEY, VALUE) VALUES (\"k2\", {\"type\":\"t\",\"v\":2})"

run_query "SELECT * FROM \`default\` WHERE type = \"t\""
