#!/usr/bin/env python3

"""
Synthetic load generator for an NGINX instance.

Sends HTTP requests to multiple paths with varying methods, concurrency,
and keep-alive behavior to exercise the OTel NGINX receiver metrics:
nginx.connections_accepted, nginx.connections_handled,
nginx.connections_current, and nginx.requests.

No external dependencies — uses only the standard library.

Usage:
    python nginx_load_generator.py [--url http://localhost:80] [--rate 100] [--workers 4]

    --rate     Target requests per second (total across all workers). Default: 100.
    --workers  Number of concurrent threads. Default: 4.
"""

import argparse
import random
import threading
import time
import urllib.request
import urllib.error


PATHS = ["/", "/index.html", "/about", "/api/data", "/images/logo.png", "/missing"]


def worker(base_url, rate_per_worker, stop_event, stats):
    interval = 1.0 / rate_per_worker if rate_per_worker > 0 else 0
    local_ok = 0
    local_err = 0

    while not stop_event.is_set():
        path = random.choice(PATHS)
        url = f"{base_url}{path}"
        try:
            req = urllib.request.Request(url)
            with urllib.request.urlopen(req, timeout=5) as resp:
                resp.read()
            local_ok += 1
        except (urllib.error.URLError, urllib.error.HTTPError, OSError):
            local_err += 1

        if interval:
            time.sleep(interval)

    stats["ok"] += local_ok
    stats["err"] += local_err


def run(base_url, rate, num_workers):
    print(f"Target: {base_url}")
    print(f"Rate: ~{rate} req/s across {num_workers} workers  (Ctrl+C to stop)\n")

    rate_per_worker = max(1, rate // num_workers)
    stop_event = threading.Event()
    stats = {"ok": 0, "err": 0}

    threads = []
    for _ in range(num_workers):
        t = threading.Thread(target=worker, args=(base_url, rate_per_worker, stop_event, stats))
        t.daemon = True
        t.start()
        threads.append(t)

    start = time.monotonic()
    try:
        while True:
            time.sleep(5)
            elapsed = time.monotonic() - start
            total = stats["ok"] + stats["err"]
            print(f"  {elapsed:.0f}s  |  ~{total} reqs  |  errors: {stats['err']}")
    except KeyboardInterrupt:
        stop_event.set()
        for t in threads:
            t.join(timeout=2)
        elapsed = time.monotonic() - start
        total = stats["ok"] + stats["err"]
        print(f"\nDone. {total} requests in {elapsed:.1f}s ({total / elapsed:.0f} req/s), errors: {stats['err']}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="NGINX synthetic load generator")
    parser.add_argument("--url", default="http://localhost:80")
    parser.add_argument("--rate", type=int, default=100, help="Target req/sec")
    parser.add_argument("--workers", type=int, default=4, help="Concurrent threads")
    args = parser.parse_args()

    run(args.url, args.rate, args.workers)
