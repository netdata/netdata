#!/usr/bin/env python3
"""SNMP trap profile YAML emitter."""

# Groups enriched per-OID JSON records by inferred vendor and emits the shipped
# trap-profile YAML files. Unknown enterprise PENs are kept under
# enterprise-<N> buckets so extraction never silently drops traps.

from __future__ import annotations

import argparse
import json
import logging
import os
import re
import sys
from collections import OrderedDict
from typing import Any, Dict, List, Tuple

import yaml

from iana_pens import default_table as _iana_default_table


# --------------------------------------------------------------------------
# Enterprise-number lookup -- AUTHORITATIVE from IANA's enterprise-numbers
# registry. The bundled file is parsed at startup; an unknown PEN
# (not in IANA) is bucketed under ``enterprise-<N>``.
# --------------------------------------------------------------------------

ENTERPRISE_VENDORS: Dict[str, str] = _iana_default_table()
# If the bundled IANA registry file is missing (e.g. stripped checkout),
# every PEN bucketizes as ``enterprise-<N>``. We deliberately do NOT keep a
# hand-curated fallback table here: small fallback tables go stale fast and
# the previous attempt mismapped at least 3 PENs. Authoritative IANA or
# nothing.


def safe_slug(s: str) -> str:
    s = re.sub(r"[^a-zA-Z0-9._-]+", "-", s).strip("-").lower()
    return s or "unknown"


def vendor_for_oid(oid: str) -> str:
    """Return the vendor slug derived from the trap OID."""
    if not oid:
        return "unknown"
    if oid.startswith(("1.3.6.1.2.1.", "1.3.6.1.6.3.")):
        return "standard"
    if oid.startswith("1.0.8802."):
        # IEEE 802.1AB-2005 (LLDP) and related IEEE Std MIBs.
        return "ieee-lldp"
    if oid.startswith("1.3.111."):
        # IEEE 802 standards published under IEEE's IANA-assigned subtree.
        return "ieee-802"
    if oid.startswith("1.3.6.1.4.1."):
        rest = oid[len("1.3.6.1.4.1."):]
        first = rest.split(".", 1)[0]
        if first in ENTERPRISE_VENDORS:
            return ENTERPRISE_VENDORS[first]
        return f"enterprise-{first}"
    # OIDs outside the expected SNMP universe — bucket them so we don't
    # lose them.
    head = oid.split(".", 1)[0]
    return safe_slug(f"oid-{head}")


# --------------------------------------------------------------------------
# Varbind shaping for the profile YAML
# --------------------------------------------------------------------------


def slim_varbind(vb: Dict[str, Any]) -> Dict[str, Any]:
    """Return the slim shipping form of a single varbind."""
    rec: Dict[str, Any] = OrderedDict()
    if vb.get("oid"):
        rec["oid"] = vb["oid"]
    syntax = vb.get("syntax")
    if syntax:
        rec["type"] = syntax
    enum = vb.get("enum")
    if isinstance(enum, dict) and enum:
        rec["enum"] = {str(k): v for k, v in enum.items()}
    if vb.get("syntax_constraints"):
        rec["constraints"] = vb["syntax_constraints"]
    return dict(rec)


def collect_vendor_varbinds(
    recs: List[Dict[str, Any]],
) -> Tuple[Dict[str, Dict[str, Any]], List[Tuple[str, Dict[str, Any]]]]:
    """Build the per-file deduped varbind table."""
    table: Dict[str, Dict[str, Any]] = {}
    inline: List[Tuple[str, Dict[str, Any]]] = []
    for rec in recs:
        trap_oid = rec.get("oid") or ""
        for vb in rec.get("varbinds") or []:
            if not isinstance(vb, dict):
                continue
            name = vb.get("name")
            slim = slim_varbind(vb)
            if not slim.get("oid") or not slim.get("type"):
                # Unresolved reference: no usable varbind metadata. Drop it
                # rather than emit an entry that violates the schema (which
                # requires BOTH ``oid`` and ``type`` on every varbind entry).
                continue
            if not name:
                # Anonymous varbind: cannot enter the table, ships inline.
                inline.append((trap_oid, slim))
                continue
            existing = table.get(name)
            if existing is None:
                table[name] = slim
            elif existing != slim:
                # Same name, different shape. Keep the first; mark this
                # occurrence inline so we never lose information.
                inline.append((trap_oid, dict(slim, name=name)))
    return table, inline


def build_profile_entry(
    rec: Dict[str, Any],
    inline_overrides_by_oid: Dict[str, List[Dict[str, Any]]],
) -> Dict[str, Any]:
    entry: Dict[str, Any] = OrderedDict()
    entry["oid"] = rec.get("oid")
    # Trap name is MIB-qualified in canonical SMI form (<MIB-MODULE>::<symbol>)
    # so that the slug is globally unique even when vendors reuse the same
    # symbolic name across product-line MIB modules. Falls back to the bare
    # symbol only when the source MIB is unknown (very rare).
    symbol = rec.get("name")
    mib = rec.get("mib")
    if symbol and mib:
        entry["name"] = f"{mib}::{symbol}"
    elif symbol:
        entry["name"] = symbol
    entry["category"] = rec.get("category") or "unknown"
    entry["severity"] = rec.get("severity") or "info"
    if rec.get("description"):
        entry["description"] = rec["description"]
    if rec.get("trap_status"):
        entry["status"] = rec["trap_status"]

    # Reference-list form: each varbind is just its bare symbol. Plugin
    # resolves via the file-level ``varbinds:`` table. Anonymous or
    # conflicting varbinds fall back to inline-dict shape on the trap entry.
    # Varbinds whose record carries no usable ``oid`` OR no ``syntax``
    # (a.k.a. ``type`` in the slim form) are dropped here too, matching the
    # table-build rule in collect_vendor_varbinds() — the profile-format
    # schema requires both fields and the plugin can't render a dangling
    # reference.
    refs: List[Any] = []
    for vb in rec.get("varbinds") or []:
        if not isinstance(vb, dict):
            continue
        if not vb.get("oid") or not vb.get("syntax"):
            continue
        if vb.get("name"):
            refs.append(vb["name"])
    inline = inline_overrides_by_oid.get(rec.get("oid") or "") or []
    for vb in inline:
        refs.append(vb)
    if refs:
        entry["varbinds"] = refs
    return dict(entry)


# --------------------------------------------------------------------------
# Aggregation + YAML emission
# --------------------------------------------------------------------------


def main() -> int:
    p = argparse.ArgumentParser(description="Emit per-vendor SNMP profile YAML files.")
    p.add_argument("--in-dir", default="output/enriched")
    p.add_argument("--out-dir", default="output/profiles")
    p.add_argument("--report", default="output/profile-emit-report.json")
    p.add_argument("--catalogue", default="output/profiles/catalogue.json",
                   help="Operator-facing index of vendors -> trap_count / mib_count / sample_traps.")
    p.add_argument("--log-level", default="INFO")
    args = p.parse_args()

    logging.basicConfig(
        level=getattr(logging, args.log_level.upper(), logging.INFO),
        format="%(asctime)s %(levelname)s %(message)s",
    )

    by_vendor: Dict[str, List[Dict[str, Any]]] = {}
    n_in = 0
    n_skipped = 0

    if not os.path.isdir(args.in_dir):
        logging.error("input dir not found: %s", args.in_dir)
        return 2

    for fn in sorted(os.listdir(args.in_dir)):
        if not fn.endswith(".json"):
            continue
        path = os.path.join(args.in_dir, fn)
        rec = None
        try:
            with open(path) as f:
                rec = json.load(f)
        except Exception as exc:
            logging.warning("skip %s: %s", path, exc)
            n_skipped += 1
        if rec is None:
            continue
        n_in += 1
        oid = rec.get("oid") or ""
        vendor = vendor_for_oid(oid)
        by_vendor.setdefault(vendor, []).append(rec)

    os.makedirs(args.out_dir, exist_ok=True)
    vendor_counts: Dict[str, int] = {}
    catalogue: Dict[str, Dict[str, Any]] = {}
    for vendor, recs in sorted(by_vendor.items()):
        recs_sorted = sorted(recs, key=lambda r: (r.get("oid") or "", r.get("name") or ""))
        vb_table, inline_pairs = collect_vendor_varbinds(recs_sorted)
        inline_by_oid: Dict[str, List[Dict[str, Any]]] = {}
        for oid, vb in inline_pairs:
            inline_by_oid.setdefault(oid, []).append(vb)
        entries = [build_profile_entry(r, inline_by_oid) for r in recs_sorted]
        out_path = os.path.join(args.out_dir, f"{vendor}.yaml")
        mibs = sorted({r.get("mib") for r in recs_sorted if r.get("mib")})

        def qualified_name(r: Dict[str, Any]) -> str:
            sym = r.get("name")
            mib = r.get("mib")
            if sym and mib:
                return f"{mib}::{sym}"
            return sym or ""
        sample_traps = [
            {"oid": r.get("oid"), "name": qualified_name(r)}
            for r in recs_sorted[:5]
        ]
        catalogue[vendor] = {
            "file": f"{vendor}.yaml",
            "trap_count": len(entries),
            "varbind_count": len(vb_table),
            "mib_count": len(mibs),
            "mibs": mibs,
            "sample_traps": sample_traps,
        }
        with open(out_path, "w") as f:
            f.write(
                "# SNMP trap profile - vendor: {v}\n"
                "# Generated by tools/snmp-traps-profile-gen.  Do not edit by hand;\n"
                "# operator overrides belong in /etc/netdata/go.d/snmp.trap-profiles/.\n"
                "#\n"
                "# Schema reference: src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md\n"
                "# Design spec:     .agents/sow/specs/snmp-traps/netdata.md\n"
                "#   varbinds: name-keyed table of varbind metadata for this vendor\n"
                "#   traps:    each entry references varbinds by name from the table above\n".format(v=vendor)
            )
            if vb_table:
                # Sort varbind names for deterministic output.  Plain dicts
                # (Python 3.7+ preserves insertion order) so yaml.safe_dump
                # serializes cleanly without an OrderedDict representer.
                ordered_vbs: Dict[str, Dict[str, Any]] = {
                    name: dict(vb_table[name]) for name in sorted(vb_table)
                }
                f.write(yaml.safe_dump(
                    {"varbinds": ordered_vbs},
                    sort_keys=False,
                    allow_unicode=True,
                    default_flow_style=False,
                    width=10_000,
                ))
            f.write("traps:\n")
            for entry in entries:
                txt = yaml.safe_dump(
                    [entry],
                    sort_keys=False,
                    allow_unicode=True,
                    default_flow_style=False,
                    width=10_000,
                )
                for line in txt.splitlines():
                    f.write("  " + line + "\n")
        vendor_counts[vendor] = len(entries)
        logging.info("wrote %s (%d traps, %d shared varbinds)",
                     out_path, len(entries), len(vb_table))

    with open(args.catalogue, "w") as f:
        json.dump(catalogue, f, indent=2, sort_keys=True)
    logging.info("wrote catalogue %s (%d vendors)", args.catalogue, len(catalogue))

    report = {
        "in_dir": args.in_dir,
        "out_dir": args.out_dir,
        "catalogue": args.catalogue,
        "records_in": n_in,
        "records_skipped": n_skipped,
        "vendors": len(by_vendor),
        "per_vendor_counts": vendor_counts,
    }
    with open(args.report, "w") as f:
        json.dump(report, f, indent=2, sort_keys=True)
    print(json.dumps(report, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    sys.exit(main())
