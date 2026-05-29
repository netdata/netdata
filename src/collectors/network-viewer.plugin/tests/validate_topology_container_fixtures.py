#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later

import json
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[4]
FIXTURE_DIR = Path(__file__).resolve().parent / "fixtures" / "topology"

GROUP_BY = ["process_name", "pid", "container"]

NEW_COLUMNS = {
    "container_name": ("string", "group_key"),
    "cgroup_status": ("string", "attribute"),
    "cgroup_path": ("string", "attribute"),
    "cgroup_name": ("string", "attribute"),
    "orchestrator": ("string", "attribute"),
    "k8s_pod_name": ("string", "attribute"),
    "k8s_namespace": ("string", "attribute"),
    "k8s_workload": ("string", "attribute"),
    "docker_container_name": ("string", "attribute"),
    "docker_image": ("string", "attribute"),
    "systemd_unit_name": ("string", "attribute"),
}

SET_AGGREGATION_COLUMNS = {
    "machine_guid",
    "hostname",
    "container_name",
    "cgroup_status",
    "cgroup_path",
    "cgroup_name",
    "orchestrator",
    "k8s_pod_name",
    "k8s_namespace",
    "k8s_workload",
    "docker_container_name",
    "docker_image",
    "systemd_unit_name",
}

SEARCH_LABEL_KEYS = {
    "process",
    "container_name",
    "cgroup_status",
    "orchestrator",
    "k8s_pod_name",
    "k8s_namespace",
    "k8s_workload",
    "docker_container_name",
    "docker_image",
    "systemd_unit_name",
}

SCOPES = {
    "process_name": ["process"],
    "pid": ["pid", "net_ns_inode"],
    "container": ["container_name"],
}


def load_fixture(name):
    with (FIXTURE_DIR / name).open("r", encoding="utf-8") as fp:
        return json.load(fp)


def decode_column(table, column_id):
    ids = [column["id"] for column in table["columns"]]
    try:
        idx = ids.index(column_id)
    except ValueError as err:
        raise AssertionError(f"missing column {column_id}") from err

    encoding = table["values"][idx]
    codec = encoding["codec"]
    if codec == "const":
        return [encoding["value"]] * table["rows"]
    if codec == "values":
        values = encoding["values"]
    elif codec == "dict":
        values = [encoding["values"][idx] for idx in encoding["indexes"]]
    else:
        raise AssertionError(f"unsupported codec {codec} for column {column_id}")

    if len(values) != table["rows"]:
        raise AssertionError(f"column {column_id} length mismatch")
    return values


def actor_rows(payload):
    actors = payload["data"]["actors"]
    return {column["id"]: decode_column(actors, column["id"]) for column in actors["columns"]}


def label_rows(payload):
    table = payload["data"]["tables"]["actor"]["actor_labels"]["table"]
    return {column["id"]: decode_column(table, column["id"]) for column in table["columns"]}


def assert_contract(payload):
    data = payload["data"]
    assert data["view"]["group_by"] == GROUP_BY

    process_scopes = data["types"]["actor_types"]["process"]["aggregation_scopes"]
    assert process_scopes == ["process_name", "pid"]
    container_scopes = data["types"]["actor_types"]["container"]["aggregation_scopes"]
    assert container_scopes == ["container"]
    for actor_type in ("process", "container"):
        search = data["types"]["actor_types"][actor_type]["search"]
        assert SEARCH_LABEL_KEYS.issubset(set(search["label_keys"]))

    actor_columns = {column["id"]: column for column in data["actors"]["columns"]}
    for name, (column_type, role) in NEW_COLUMNS.items():
        column = actor_columns[name]
        assert column["type"] == column_type
        assert column["role"] == role
        assert column.get("nullable") is True

    for name in SET_AGGREGATION_COLUMNS:
        column = actor_columns[name]
        assert column["aggregation"] == "set"

    aggregation_scopes = data["types"]["aggregation_scopes"]
    for scope, columns in SCOPES.items():
        assert aggregation_scopes[scope]["columns"] == columns
        assert aggregation_scopes[scope]["evidence_policy"] == "preserve"


def row_by_pid(rows, pid):
    pids = rows["pid"]
    try:
        idx = pids.index(pid)
    except ValueError as err:
        raise AssertionError(f"missing pid {pid}") from err
    return {key: values[idx] for key, values in rows.items()}


def assert_with_containers(payload):
    assert_contract(payload)
    rows = actor_rows(payload)

    assert row_by_pid(rows, 101)["orchestrator"] == "docker"
    assert row_by_pid(rows, 101)["cgroup_status"] == "known"
    assert row_by_pid(rows, 101)["container_name"] == "demo-nginx"
    assert row_by_pid(rows, 101)["cgroup_name"] == "demo-nginx"
    assert row_by_pid(rows, 201)["orchestrator"] == "k8s"
    assert row_by_pid(rows, 201)["k8s_pod_name"] == "web-7b9d5f4c5-q2lrz"
    assert row_by_pid(rows, 201)["k8s_namespace"] == "demo"
    assert row_by_pid(rows, 201)["k8s_workload"] == "web"
    assert row_by_pid(rows, 301)["orchestrator"] == "systemd"
    assert row_by_pid(rows, 301)["container_name"] == "sshd.service"
    assert row_by_pid(rows, 301)["systemd_unit_name"] == "sshd.service"
    assert row_by_pid(rows, 401)["orchestrator"] == "lxc"
    assert row_by_pid(rows, 501)["orchestrator"] == "kvm"
    assert row_by_pid(rows, 601)["orchestrator"] == "host_root"
    assert row_by_pid(rows, 601)["cgroup_status"] == "host_root"
    assert row_by_pid(rows, 601)["container_name"] == "bash"
    assert row_by_pid(rows, 601)["cgroup_name"] is None

    labels = label_rows(payload)
    assert len(labels["key"]) == 0


def assert_zero_containers(payload):
    assert_contract(payload)
    rows = actor_rows(payload)
    for column in NEW_COLUMNS:
        if column == "container_name":
            continue
        assert all(value is None for value in rows[column]), column


def assert_whitelist(payload):
    assert_contract(payload)
    labels = label_rows(payload)
    assert len(labels["key"]) > 0
    assert {"team", "app"}.issubset(set(labels["key"]))
    assert all(source == "cgroups" for source in labels["source"])
    assert all(kind == "label" for kind in labels["kind"])


def assert_mixed(payload):
    assert_contract(payload)
    rows = actor_rows(payload)
    row_count = len(rows["pid"])
    host_root = 0
    enriched = 0
    sparse_columns = [
        "cgroup_path",
        "cgroup_name",
        "k8s_pod_name",
        "k8s_namespace",
        "k8s_workload",
        "systemd_unit_name",
    ]

    for idx in range(row_count):
        assert rows["pid"][idx] is not None
        assert rows["display_name"][idx]

        if rows["orchestrator"][idx] == "host_root":
            assert rows["cgroup_status"][idx] == "host_root"
            host_root += 1
        else:
            assert rows["cgroup_status"][idx] == "known"
            enriched += 1

        for column in sparse_columns:
            if rows[column][idx] is None:
                assert rows["pid"][idx] is not None
                assert rows["display_name"][idx]

    assert host_root >= 3
    assert enriched >= 3


def main():
    checks = {
        "with-containers.json": assert_with_containers,
        "zero-containers.json": assert_zero_containers,
        "whitelist-allow-all.json": assert_whitelist,
        "mixed-containers.json": assert_mixed,
    }

    for fixture, check in checks.items():
        check(load_fixture(fixture))

    print(f"validated {len(checks)} topology container fixtures")


if __name__ == "__main__":
    try:
        main()
    except AssertionError as err:
        print(f"fixture validation failed: {err}", file=sys.stderr)
        sys.exit(1)
