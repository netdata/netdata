#!/usr/bin/env python3
"""
Example: Push system metrics to Netdata Alternative UI

This script collects system metrics (CPU, memory, disk, network) and
pushes them to the alternative UI server.

Requirements:
    pip install psutil requests

Usage:
    python push_metrics.py [--url URL] [--node-id NODE_ID] [--interval SECONDS]
"""

import argparse
import json
import os
import platform
import socket
import time
from datetime import datetime

try:
    import psutil
    import requests
except ImportError:
    print("Please install required packages: pip install psutil requests")
    exit(1)


def get_cpu_metrics():
    """Get CPU usage per core."""
    cpu_percent = psutil.cpu_percent(percpu=True)
    return {
        "id": "system.cpu",
        "type": "system",
        "name": "cpu",
        "title": "CPU Usage",
        "units": "%",
        "family": "cpu",
        "context": "system.cpu",
        "chart_type": "stacked",
        "priority": 100,
        "dimensions": [
            {"id": f"core{i}", "name": f"Core {i}", "value": v}
            for i, v in enumerate(cpu_percent)
        ]
    }


def get_memory_metrics():
    """Get memory usage."""
    mem = psutil.virtual_memory()
    return {
        "id": "system.memory",
        "type": "system",
        "name": "memory",
        "title": "Memory Usage",
        "units": "MiB",
        "family": "memory",
        "context": "system.memory",
        "chart_type": "stacked",
        "priority": 200,
        "dimensions": [
            {"id": "used", "name": "Used", "value": mem.used / (1024 * 1024)},
            {"id": "cached", "name": "Cached", "value": mem.cached / (1024 * 1024)},
            {"id": "buffers", "name": "Buffers", "value": mem.buffers / (1024 * 1024)},
            {"id": "free", "name": "Free", "value": mem.available / (1024 * 1024)},
        ]
    }


def get_disk_io_metrics():
    """Get disk I/O metrics."""
    try:
        disk_io = psutil.disk_io_counters()
        return {
            "id": "system.disk_io",
            "type": "system",
            "name": "disk_io",
            "title": "Disk I/O",
            "units": "KiB/s",
            "family": "disk",
            "context": "system.disk_io",
            "chart_type": "area",
            "priority": 300,
            "dimensions": [
                {"id": "read", "name": "Read", "value": disk_io.read_bytes / 1024, "algorithm": "incremental"},
                {"id": "write", "name": "Write", "value": disk_io.write_bytes / 1024, "algorithm": "incremental"},
            ]
        }
    except Exception:
        return None


def get_network_metrics():
    """Get network I/O metrics."""
    try:
        net_io = psutil.net_io_counters()
        return {
            "id": "system.network",
            "type": "system",
            "name": "network",
            "title": "Network Traffic",
            "units": "KiB/s",
            "family": "network",
            "context": "system.network",
            "chart_type": "area",
            "priority": 400,
            "dimensions": [
                {"id": "received", "name": "Received", "value": net_io.bytes_recv / 1024, "algorithm": "incremental"},
                {"id": "sent", "name": "Sent", "value": net_io.bytes_sent / 1024, "algorithm": "incremental"},
            ]
        }
    except Exception:
        return None


def get_load_average():
    """Get system load average."""
    try:
        load = os.getloadavg()
        return {
            "id": "system.load",
            "type": "system",
            "name": "load",
            "title": "System Load Average",
            "units": "load",
            "family": "load",
            "context": "system.load",
            "chart_type": "line",
            "priority": 150,
            "dimensions": [
                {"id": "load1", "name": "1 min", "value": load[0]},
                {"id": "load5", "name": "5 min", "value": load[1]},
                {"id": "load15", "name": "15 min", "value": load[2]},
            ]
        }
    except (OSError, AttributeError):
        return None


def collect_metrics():
    """Collect all system metrics."""
    charts = []

    cpu = get_cpu_metrics()
    if cpu:
        charts.append(cpu)

    mem = get_memory_metrics()
    if mem:
        charts.append(mem)

    load = get_load_average()
    if load:
        charts.append(load)

    disk = get_disk_io_metrics()
    if disk:
        charts.append(disk)

    net = get_network_metrics()
    if net:
        charts.append(net)

    return charts


def push_metrics(url, node_id, node_name, api_key=None):
    """Push metrics to the alternative UI server."""
    charts = collect_metrics()

    payload = {
        "node_id": node_id,
        "node_name": node_name,
        "hostname": socket.gethostname(),
        "os": f"{platform.system()} {platform.release()}",
        "timestamp": int(time.time() * 1000),
        "charts": charts,
    }

    headers = {"Content-Type": "application/json"}
    if api_key:
        headers["X-API-Key"] = api_key

    try:
        response = requests.post(
            f"{url}/api/v1/push",
            data=json.dumps(payload),
            headers=headers,
            timeout=5
        )
        response.raise_for_status()
        return True
    except requests.RequestException as e:
        print(f"Failed to push metrics: {e}")
        return False


def main():
    parser = argparse.ArgumentParser(description="Push system metrics to Netdata Alternative UI")
    parser.add_argument("--url", default="http://localhost:19998", help="Server URL")
    parser.add_argument("--node-id", default=socket.gethostname(), help="Node ID")
    parser.add_argument("--node-name", default=None, help="Node display name")
    parser.add_argument("--api-key", default=None, help="API key for authentication")
    parser.add_argument("--interval", type=int, default=1, help="Collection interval in seconds")
    args = parser.parse_args()

    node_name = args.node_name or args.node_id

    print(f"Pushing metrics to {args.url}")
    print(f"Node ID: {args.node_id}")
    print(f"Node Name: {node_name}")
    print(f"Interval: {args.interval}s")
    print()

    while True:
        success = push_metrics(args.url, args.node_id, node_name, args.api_key)
        if success:
            print(f"[{datetime.now().strftime('%H:%M:%S')}] Metrics pushed successfully")
        time.sleep(args.interval)


if __name__ == "__main__":
    main()
