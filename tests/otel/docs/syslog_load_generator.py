#!/usr/bin/env python3

"""
Synthetic syslog message generator for testing the OTel syslog receiver.

Sends RFC 5424 or RFC 3164 formatted syslog messages over UDP or TCP to
exercise the OTel syslog receiver pipeline.

No external dependencies -- uses only the standard library.

Usage:
    python syslog_load_generator.py [--host localhost] [--port 54526] [--rate 20] \
        [--protocol udp] [--format rfc5424]

    --protocol  Transport protocol: udp or tcp. Default: udp.
    --format    Syslog format: rfc5424 or rfc3164. Default: rfc5424.
    --rate      Messages per second (approximate). Default: 20.
"""

import argparse
import datetime
import random
import socket
import time


FACILITIES = {
    "kern": 0, "user": 1, "mail": 2, "daemon": 3,
    "auth": 4, "syslog": 5, "local0": 16, "local1": 17,
}
SEVERITIES = {
    "emerg": 0, "alert": 1, "crit": 2, "err": 3,
    "warning": 4, "notice": 5, "info": 6, "debug": 7,
}

HOSTNAMES = ["webserver01", "dbserver02", "appserver03", "cache01"]
APP_NAMES = ["nginx", "postgres", "myapp", "redis", "sshd"]

MESSAGES = [
    "Connection accepted from 10.0.0.{rand}",
    "Request completed in {dur}ms",
    "Authentication succeeded for user admin",
    "Authentication failed for user root from 192.168.1.{rand}",
    "Configuration reloaded successfully",
    "Worker process started, pid={pid}",
    "Disk usage at {pct}% on /data",
    "Slow query detected: {dur}ms",
    "Rate limit exceeded for client 10.0.0.{rand}",
    "Health check passed",
    "Certificate renewal scheduled",
    "Cache eviction: {cnt} keys removed",
    "Backup completed: {cnt} files, {size}MB",
    "Service restarted after crash",
]

SEV_WEIGHTS = [
    ("info", 40), ("notice", 15), ("warning", 15), ("err", 10),
    ("debug", 10), ("crit", 5), ("alert", 3), ("emerg", 2),
]
SEV_CHOICES = [s for s, w in SEV_WEIGHTS for _ in range(w)]


def fill_message(template):
    return template.format(
        rand=random.randint(1, 254),
        dur=random.randint(5, 3000),
        pid=random.randint(1000, 65000),
        pct=random.randint(50, 99),
        cnt=random.randint(10, 5000),
        size=random.randint(1, 500),
    )


def make_rfc5424(facility, severity, hostname, appname, msg):
    pri = facility * 8 + severity
    ts = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%f")[:-3] + "Z"
    pid = random.randint(1000, 65000)
    msgid = f"ID{random.randint(100, 999)}"
    return f"<{pri}>1 {ts} {hostname} {appname} {pid} {msgid} - {msg}"


def make_rfc3164(facility, severity, hostname, appname, msg):
    pri = facility * 8 + severity
    ts = datetime.datetime.now(datetime.timezone.utc).strftime("%b %d %H:%M:%S")
    pid = random.randint(1000, 65000)
    return f"<{pri}>{ts} {hostname} {appname}[{pid}]: {msg}"


def run(host, port, rate, protocol, fmt):
    formatter = make_rfc5424 if fmt == "rfc5424" else make_rfc3164

    if protocol == "udp":
        sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    else:
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.connect((host, port))

    print(f"Target: {protocol.upper()}://{host}:{port}")
    print(f"Format: {fmt.upper()}")
    print(f"Rate: ~{rate} msg/s  (Ctrl+C to stop)\n")

    interval = 1.0 / rate if rate > 0 else 0
    ops = 0
    start = time.monotonic()

    try:
        while True:
            sev_name = random.choice(SEV_CHOICES)
            fac_name = random.choice(list(FACILITIES.keys()))
            hostname = random.choice(HOSTNAMES)
            appname = random.choice(APP_NAMES)
            msg = fill_message(random.choice(MESSAGES))

            line = formatter(
                FACILITIES[fac_name],
                SEVERITIES[sev_name],
                hostname,
                appname,
                msg,
            )
            data = (line + "\n").encode("utf-8")

            if protocol == "udp":
                sock.sendto(data, (host, port))
            else:
                sock.sendall(data)

            ops += 1
            if ops % 200 == 0:
                elapsed = time.monotonic() - start
                print(f"  {ops} msgs in {elapsed:.1f}s  ({ops / elapsed:.0f} msg/s)")

            if interval:
                time.sleep(interval)

    except KeyboardInterrupt:
        elapsed = time.monotonic() - start
        print(f"\nDone. {ops} messages in {elapsed:.1f}s ({ops / elapsed:.0f} msg/s)")
    finally:
        sock.close()


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Syslog synthetic load generator")
    parser.add_argument("--host", default="localhost")
    parser.add_argument("--port", type=int, default=54526)
    parser.add_argument("--rate", type=int, default=20, help="Messages per second")
    parser.add_argument("--protocol", choices=["udp", "tcp"], default="udp")
    parser.add_argument("--format", choices=["rfc5424", "rfc3164"], default="rfc5424", dest="fmt")
    args = parser.parse_args()

    run(args.host, args.port, args.rate, args.protocol, args.fmt)
