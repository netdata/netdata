"""
Simple Prometheus metrics endpoint for testing an OpenTelemetry collector pipeline.

Exposes metrics on localhost:9090/metrics that the OTel collector can scrape.

Requirements:
    pip install prometheus-client

Usage:
    python test_metrics_server.py
"""

import random
import time
import threading
from prometheus_client import start_http_server, Counter, Gauge, Histogram, Summary

# Counter - monotonically increasing value
REQUEST_COUNT = Counter(
    "myapp_requests_total",
    "Total number of requests",
    ["method", "endpoint"],
)

# Gauge - value that can go up and down
TEMPERATURE = Gauge(
    "myapp_temperature_celsius",
    "Current temperature in celsius",
)

CPU_USAGE = Gauge(
    "myapp_cpu_usage_percent",
    "Simulated CPU usage percentage",
)

ACTIVE_CONNECTIONS = Gauge(
    "myapp_active_connections",
    "Number of active connections",
)

# Histogram - observations bucketed by size
REQUEST_LATENCY = Histogram(
    "myapp_request_duration_seconds",
    "Request latency in seconds",
    ["endpoint"],
    buckets=[0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0],
)

# Summary - similar to histogram but with quantiles
RESPONSE_SIZE = Summary(
    "myapp_response_size_bytes",
    "Response size in bytes",
)


def simulate_metrics():
    """Generate fake metric values in a loop."""
    endpoints = ["/api/users", "/api/orders", "/api/health", "/api/products"]
    methods = ["GET", "POST", "PUT", "DELETE"]

    while True:
        # Bump request counters
        for _ in range(random.randint(1, 5)):
            method = random.choice(methods)
            endpoint = random.choice(endpoints)
            REQUEST_COUNT.labels(method=method, endpoint=endpoint).inc()

        # Update gauges
        TEMPERATURE.set(20 + random.uniform(-5, 15))
        CPU_USAGE.set(random.uniform(10, 95))
        ACTIVE_CONNECTIONS.set(random.randint(0, 200))

        # Record histogram observations
        for endpoint in endpoints:
            REQUEST_LATENCY.labels(endpoint=endpoint).observe(random.expovariate(1 / 0.3))

        # Record summary observations
        RESPONSE_SIZE.observe(random.randint(100, 50000))

        time.sleep(5)


if __name__ == "__main__":
    start_http_server(9090)
    print("Prometheus metrics server running on http://localhost:9090/metrics")
    print("Press Ctrl+C to stop.")

    thread = threading.Thread(target=simulate_metrics, daemon=True)
    thread.start()

    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        print("\nStopped.")
