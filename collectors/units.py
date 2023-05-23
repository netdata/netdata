"""
Netdata Unit converter tool.
"""

import csv
import sys

#
# Mapping Rules:
# * Map to pre-defined UCUM units as much as possible
# * Metrics that count things and can be expressed as "3 apples" in a sentence,
#   should have the singular thing to be counted in curly bracket. Like
#   "{apple}".
# * Other dimensionless values, like a state should map to "1". E.g. you
#   cannot say "1 status" or "0 bool", so no unit should be displayed after
#   the value.
#

#
# Mapping from existing (constituent) units to UCUM units
#
mapping = {
    "%": "%",
    "% of time working": "%",
    "Ah": "A.h",
    "Ampere": "A",
    "Amps": "A",
    "B": "By",
    "Celsius": "Cel",
    "Fahrenheit": "[degF]",
    "GiB": "GiBy",
    "Hz": "Hz",
    "Joule": "J",
    "KB": "KiBy",
    "KiB": "KiBy",
    "MHz": "MHz",
    "Mbps": "Mibit/s",
    "MB": "MiBy",
    "MiB": "MiBy",
    "Minute": "min",
    "Minutes": "min",
    "Percent": "%",
    "RPM": "{rotation}/min",
    "Status": "1",  # bit field, dimensionless but no suffix
    "V": "V",
    "Volts": "V",
    "Watt": "W",
    "Watts": "W",
    "Wh": "W.h",
    "active connections": "{connection}",
    "bool": "1",
    "boolean": "1",
    "byte": "By",
    "bytes": "By",
    "celsius": "Cel",
    "children": "{child}",
    "context switches": "{context switch}",
    "count": "1",
    "cpu time": "1",
    "current threads": "{thread}",
    "cycle count": "{cycle}",
    "dBm": "dB[mW]",
    "dests": "{destination}",
    "difference": "1",
    "entries": "{entry}",
    "expired": "{object}",
    "failed disks": "{disk}",
    "failed servers": "{server}",
    "flushes": "{flush}",
    "health servers": "{server}",
    "hours": "h",
    "is degraded": "1",
    "kilobit": "Kibit",
    "kilobits": "Kibit",
    "kilobyte": "KiBy",
    "kilobytes": "KiBy",
    "merged operation": "{operation}",
    "microseconds": "us",
    "microseconds lost": "us",
    "milisecondds": "ms",
    "miliseconds": "ms",
    "milliseconds": "ms",
    "misses": "{miss}",
    "ms": "ms",
    "ns": "ns",
    "num": "1",
    "nuked": "{object}",
    "number": "1",
    "octet": "By",
    "open files": "{file}",
    "open pipes": "{pipe}",
    "ops": "{operation}",
    "percent": "%",
    "percentage": "%",
    "pps": "{packet}/s",
    "prefetches": "{prefetch}",
    "processes": "{process}",
    "queries": "{query}",
    "replies": "{reply}",
    "retries": "{retry}",
    "s": "s",
    "searches": "{search}",
    "sec": "s",
    "seconds": "s",
    "status": "1",
    "switches": "{switch}",
    "temperature": "Cel",
    "total": "1",
    "unsynchronised blocks": "{block}",
    "value": "1",
    "z": "1",

    # From go.d.plugin
    "4K, 2M, 1G": "{page}",
    "attached": "{device}",
    "classes": "{class}",
    "deposited": "{page}",
    "difficulty": "1",
    "dispatches": "{dispatch}",
    "frequency": "/s",
    "gpa": "{modification}",
    "hmode": "1",
    "indexes": "{index}",
    "log2": "1",
    "message bacthes": "{batch}",
    "message batches": "{batch}",
    "millicpu": "m{cpu}",
    "ok, critical": "1",
    "pgfaults": "{page fault}",
    "pmode": "1",
    "ppm": "[ppm]",
    "queue_length": "1",
    "queued_size": "1",
    "ratio": "1",
    "running_containers": "{container}",
    "running_pod"
    "running_pods": "{pod}",
    "size": "1",  # maybe "By"?
    "state": "1",
    "stratum": "1",
    "token_requests": "{request}",
    "us": "us",
    "watches": "{watch}",
    "xid": "1",
}

#
# Units for specific metric names
#
overrides = {
    "varnish.threads_total": "{thread}",
    "dovecot.sessions": "{session}",
    "dovecot.faults": "{page fault}",
    "dovecot.logins": "{login}",
    "dovecot.lookup": "{lookup}/s",
    "mem.pgfaults": "{page fault}/s",
    "system.pgfaults": "{page fault}/s",
    "smartd_log.erase_fail_count": "{erase}",
    "smartd_log.program_fail_count": "{program}",
    "mysql.thread_cache_misses": "{thread}",
    "mysql.galera_cluster_weight": "1",
    "hdfs.load": "{concurrent file accesses}",
}


def convert_unit(metric_name: str, unit: str) -> str:
    """
    Map an old unit to a UCUM-conforming one.

    1. If the metric name exists in `overrides`, use that.
    2. Otherwise:
      - Split the unit in components separated by a slash
      - Strip the whitespace.
    3. For each part:
      - Check if the unit appears in `mapping` and then use that.
      - Otherwise:
        - Strip a final s to make it singular
        - Put in between curly braces.
        - Lowercase
    4. Join parts with a slash.
    """
    try:
        result = overrides[metric_name]
    except KeyError:
        parts = [part.strip() for part in unit.split("/")]
        result_parts = []
        for part in parts:
            try:
                part = mapping[part]
            except KeyError:
                if part[-1] == "s":
                    part = part[:-1]
                part = f"{{{part}}}".lower()
            result_parts.append(part)

        result = "/".join(result_parts)
    return result


def main() -> None:
    """
    Open file as CSV and print out rows with converted units for verification.

    The CSV is expected to follow the metrics.csv format.
    """
    with open(sys.argv[1], encoding="utf-8") as csvfile:
        rows = csv.reader(csvfile, delimiter=",", quotechar='"')
        for row in rows:
            if row[0] == "metric" and row[3] == "unit":
                continue
            print("-----")
            print(f"Metric: {row[0]}")
            print(f"  Scope: {row[1]}")
            print(f"  Dimensions: {row[2]}")
            old_unit = row[3]
            converted_unit = convert_unit(row[0], old_unit)
            print(f"  Unit: {old_unit} ----> {converted_unit}")
            print(f"  Description: {row[4]}")
            print(f"  Chart type: {row[5]}")
            print(f"  Labels: {row[6]}")
            print(f"  Plugin: {row[7]}")
            print(f"  Module: {row[8]}")


if __name__ == "__main__":
    main()
