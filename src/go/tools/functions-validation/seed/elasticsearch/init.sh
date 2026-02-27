#!/bin/sh
set -eu

ES_URL="${ES_URL:-http://elasticsearch:9200}"

until curl -fsS "$ES_URL/_cluster/health?wait_for_status=yellow&timeout=5s" > /dev/null; do
  sleep 2
done

curl -fsS -X PUT "$ES_URL/netdata" -H 'Content-Type: application/json' -d '{
  "settings": { "number_of_shards": 1, "number_of_replicas": 0 }
}' > /dev/null || true

{
  i=1
  while [ "$i" -le 1000 ]; do
    echo "{\"index\":{\"_index\":\"netdata\",\"_id\":\"$i\"}}"
    echo "{\"value\":$i,\"text\":\"value-$i\"}"
    i=$((i + 1))
  done
} | curl -fsS -X POST "$ES_URL/_bulk" -H 'Content-Type: application/x-ndjson' --data-binary @- > /dev/null

curl -fsS -X POST "$ES_URL/netdata/_refresh" > /dev/null
