#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Check a built plugin's info metadata without collecting host sockets."""

# Usage: python3 validate_topology_selector_metadata.py /path/to/network-viewer.plugin
#        --run-dir /path/to/an/empty/private/runtime-directory
# Requires the same jsonschema package as validate_topology_payload.py.

import argparse
import json
import os
from pathlib import Path
# This harness executes the explicitly supplied test binary.
import subprocess  # nosec B404

import jsonschema


def validate(plugin, run_dir):
    plugin = plugin.resolve(strict=True)
    run_dir = run_dir.resolve(strict=True)
    if not run_dir.is_dir() or any(run_dir.iterdir()):
        raise ValueError("--run-dir must be an existing empty directory")
    if run_dir.stat().st_uid != os.getuid():
        raise ValueError("--run-dir must belong to the current user")
    if run_dir.stat().st_mode & 0o077:
        raise ValueError("--run-dir must not be accessible to other users")

    env = dict(os.environ, NETDATA_RUN_DIR=str(run_dir))
    # Execute the caller-selected build directly; no shell or topology input forms the command.
    result = subprocess.run(  # nosec B603
        [str(plugin), "--test", "topology:network-connections"],
        input=json.dumps({"info": True}),
        text=True,
        capture_output=True,
        env=env,
        timeout=30,
        check=True,
    )
    response = json.loads(result.stdout)
    schema_file = Path(__file__).resolve().parents[3] / "plugins.d" / "FUNCTION_UI_SCHEMA.json"
    document = json.loads(schema_file.read_text(encoding="utf-8"))
    schema = {
        "$schema": document["$schema"],
        "$ref": "#/definitions/info_response",
        "definitions": document["definitions"],
    }
    jsonschema.Draft7Validator(schema).validate(response)
    assert response["status"] == 200
    assert response["type"] == "topology"
    assert "data" not in response, "info must not collect or render socket data"

    params = {param["id"]: param for param in response["required_params"]}
    assert len(params) == len(response["required_params"]), "duplicate selector IDs"
    assert list(params) == ["group_by", "__topology_mode", "sockets", "protocols", "endpoints", "labels"]

    aliases = {
        (param_id, option["id"]): option["aliases"]
        for param_id, param in params.items()
        for option in param.get("options", [])
        if "aliases" in option
    }
    assert aliases == {
        ("group_by", "container"): ["container_name"],
        ("protocols", "ipv4_tcp"): ["ipv4-tcp"],
        ("protocols", "ipv6_tcp"): ["ipv6-tcp"],
        ("protocols", "ipv4_udp"): ["ipv4-udp"],
        ("protocols", "ipv6_udp"): ["ipv6-udp"],
        ("endpoints", "by_ip"): ["by-ip"],
    }, aliases
    separators = {key: param["value_separator"] for key, param in params.items() if "value_separator" in param}
    assert separators == {"sockets": ",", "protocols": ","}, separators

    defaults = {
        key: [option["id"] for option in param.get("options", []) if option.get("defaultSelected")]
        for key, param in params.items()
    }
    assert defaults == {
        "group_by": ["process_name"],
        "__topology_mode": ["aggregated"],
        "sockets": ["listening", "inbound", "outbound"],
        "protocols": ["ipv4_tcp", "ipv6_tcp", "ipv4_udp", "ipv6_udp"],
        "endpoints": ["by_ip"],
        "labels": [],
    }, defaults
    print("PASS: real info response, accepted forms, existing defaults, and Function schema")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("plugin", type=Path)
    parser.add_argument("--run-dir", required=True, type=Path)
    args = parser.parse_args()
    validate(args.plugin, args.run_dir)


if __name__ == "__main__":
    main()
