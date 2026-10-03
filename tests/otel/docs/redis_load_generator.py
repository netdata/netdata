#!/usr/bin/env python3

"""
Synthetic load generator for a Redis instance.

Generates a mix of SET, GET, INCR, LPUSH, EXPIRE, and DEL operations
to produce visible activity in OTel Redis receiver metrics
(connections, commands processed, keyspace hits/misses, memory, net I/O).

Requirements:
    pip install redis

Usage:
    python redis_load_generator.py [--host localhost] [--port 6379] [--rate 200]

    --rate  Target operations per second (approximate). Default: 200.
"""

import argparse
import random
import string
import time

import redis


def random_key(prefix="load", n=6):
    return f"{prefix}:{random.randint(0, n)}"


def random_value(length=64):
    return "".join(random.choices(string.ascii_letters + string.digits, k=length))


def run(host, port, rate):
    r = redis.Redis(host=host, port=port, decode_responses=True)
    r.ping()
    print(f"Connected to Redis at {host}:{port}")
    print(f"Target rate: ~{rate} ops/s  (Ctrl+C to stop)\n")

    ops = 0
    start = time.monotonic()
    interval = 1.0 / rate if rate > 0 else 0

    try:
        while True:
            op = random.choices(
                ["set", "get", "incr", "lpush", "expire", "del", "miss"],
                weights=[30, 30, 10, 10, 5, 10, 5],
            )[0]

            key = random_key()

            if op == "set":
                r.set(key, random_value(random.randint(32, 512)))
            elif op == "get":
                r.get(key)
            elif op == "incr":
                r.incr(f"counter:{random.randint(0, 3)}")
            elif op == "lpush":
                r.lpush(f"list:{random.randint(0, 3)}", random_value(128))
            elif op == "expire":
                r.expire(key, random.randint(5, 30))
            elif op == "del":
                r.delete(key)
            elif op == "miss":
                # intentional miss to bump keyspace.misses
                r.get(f"nonexistent:{random.randint(1000, 9999)}")

            ops += 1

            if ops % 500 == 0:
                elapsed = time.monotonic() - start
                print(f"  {ops} ops in {elapsed:.1f}s  ({ops / elapsed:.0f} ops/s)")

            if interval:
                time.sleep(interval)

    except KeyboardInterrupt:
        elapsed = time.monotonic() - start
        print(f"\nDone. {ops} operations in {elapsed:.1f}s ({ops / elapsed:.0f} ops/s)")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Redis synthetic load generator")
    parser.add_argument("--host", default="localhost")
    parser.add_argument("--port", type=int, default=6379)
    parser.add_argument("--rate", type=int, default=200, help="Target ops/sec")
    args = parser.parse_args()

    run(args.host, args.port, args.rate)
