#!/usr/bin/env python3

"""
Synthetic log file writer for testing the OTel filelog receiver.

Writes realistic log entries (plain text or JSON) to a file that the
filelog receiver can tail. Generates a mix of log levels and formats
including timestamps, request IDs, and status codes.

No external dependencies -- uses only the standard library.

Usage:
    python filelog_load_generator.py [--file /var/log/myapp/app.log] [--rate 20] [--json]

    --file   Path to the log file to write. Default: /tmp/myapp.log.
    --rate   Log entries per second (approximate). Default: 20.
    --json   Write JSON-formatted log entries instead of plain text.
"""

import argparse
import datetime
import json
import os
import random
import time


LEVELS = ["INFO", "INFO", "INFO", "WARN", "ERROR", "DEBUG"]

MESSAGES = {
    "INFO": [
        "Request handled successfully",
        "User session created",
        "Health check passed",
        "Cache refreshed",
        "Scheduled job completed",
        "Connection established",
    ],
    "WARN": [
        "Slow query detected: 1523ms",
        "Retry attempt 2/3 for upstream service",
        "Memory usage above 80%",
        "Rate limit approaching for client",
    ],
    "ERROR": [
        "Database connection timeout after 30s",
        "Unhandled exception in /api/orders",
        "Failed to write to disk: no space left",
        "Authentication failed for user admin",
    ],
    "DEBUG": [
        "Parsed request body: 2048 bytes",
        "SQL: SELECT * FROM users WHERE id=$1",
        "Cache lookup: key=session:a1b2c3",
    ],
}

ENDPOINTS = ["/api/users", "/api/orders", "/api/health", "/api/products", "/api/auth"]
STATUS_CODES = [200, 200, 200, 201, 301, 400, 401, 403, 404, 500, 503]


def random_request_id():
    return f"{random.randint(0, 0xFFFF):04x}{random.randint(0, 0xFFFF):04x}"


def plain_line(ts, level, msg):
    return f"{ts} [{level:5s}] {msg}"


def json_line(ts, level, msg):
    entry = {
        "time": ts,
        "level": level.lower(),
        "msg": msg,
        "request_id": random_request_id(),
        "endpoint": random.choice(ENDPOINTS),
        "status": random.choice(STATUS_CODES),
        "duration_ms": round(random.expovariate(1 / 50.0), 1),
    }
    return json.dumps(entry)


def run(filepath, rate, use_json):
    os.makedirs(os.path.dirname(filepath) or ".", exist_ok=True)

    print(f"Writing to: {filepath}")
    print(f"Format: {'JSON' if use_json else 'plain text'}")
    print(f"Rate: ~{rate} entries/s  (Ctrl+C to stop)\n")

    interval = 1.0 / rate if rate > 0 else 0
    ops = 0
    start = time.monotonic()
    formatter = json_line if use_json else plain_line

    try:
        with open(filepath, "a") as f:
            while True:
                level = random.choice(LEVELS)
                msg = random.choice(MESSAGES[level])
                ts = datetime.datetime.now(datetime.timezone.utc).strftime(
                    "%Y-%m-%dT%H:%M:%S.%f"
                )[:-3] + "Z"

                line = formatter(ts, level, msg)
                f.write(line + "\n")
                f.flush()

                ops += 1
                if ops % 200 == 0:
                    elapsed = time.monotonic() - start
                    print(f"  {ops} entries in {elapsed:.1f}s  ({ops / elapsed:.0f} entries/s)")

                if interval:
                    time.sleep(interval)

    except KeyboardInterrupt:
        elapsed = time.monotonic() - start
        print(f"\nDone. {ops} entries in {elapsed:.1f}s ({ops / elapsed:.0f} entries/s)")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="File log synthetic generator")
    parser.add_argument("--file", default="/tmp/myapp.log", help="Log file path")
    parser.add_argument("--rate", type=int, default=20, help="Entries per second")
    parser.add_argument("--json", action="store_true", help="Write JSON-formatted entries")
    args = parser.parse_args()

    run(args.file, args.rate, args.json)
