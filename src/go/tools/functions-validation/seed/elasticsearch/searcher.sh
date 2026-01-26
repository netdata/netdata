#!/bin/sh
set -eu

ES_URL="${ES_URL:-http://elasticsearch:9200}"

until curl -fsS "$ES_URL/_cluster/health?wait_for_status=yellow&timeout=5s" > /dev/null; do
  sleep 2
done

while true; do
  curl -fsS -X POST "$ES_URL/netdata/_search" -H 'Content-Type: application/json' -d '{
    "size": 5000,
    "track_total_hits": true,
    "query": {
      "script_score": {
        "query": { "match_all": {} },
        "script": {
          "source": "double v = 0; for (int i = 0; i < 20000; ++i) { v += Math.sqrt(_score + i); } return v;"
        }
      }
    }
  }' > /dev/null || true
  sleep 0.2
done
