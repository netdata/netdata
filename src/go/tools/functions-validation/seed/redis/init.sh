#!/usr/bin/env bash
set -euo pipefail

REDIS_HOST="${REDIS_HOST:-redis}"
REDIS_PORT="${REDIS_PORT:-6379}"

redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" CONFIG SET slowlog-log-slower-than 0 > /dev/null
redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" CONFIG SET slowlog-max-len 1024 > /dev/null

redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" SET foo bar > /dev/null
redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" GET foo > /dev/null
redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" INCR counter > /dev/null
redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" LPUSH list a b c > /dev/null
redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" LRANGE list 0 10 > /dev/null
