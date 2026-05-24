#!/usr/bin/env python3
"""
SOW-0033/0034 - profile YAML emitter.

Reads every enriched per-OID JSON record under output/enriched/, groups
records by inferred vendor (via OID enterprise prefix lookup), and emits
one YAML file per vendor under output/profiles/ matching the schema in
``.agents/sow/specs/snmp-traps/netdata.md`` §7.

Vendor slug derivation:
  * Standard IETF/IANA trees (``1.3.6.1.2.1.*``, ``1.3.6.1.6.3.*``) -> "standard".
  * Enterprise sub-tree ``1.3.6.1.4.1.<N>.*``: ``<N>`` is looked up in a
    static enterprise-number table (small subset for the vendors we care
    about).  Anything not in the table is bucketed under ``enterprise-<N>``
    so nothing is silently dropped.
"""

from __future__ import annotations

import argparse
import json
import logging
import os
import re
import sys
from collections import OrderedDict
from typing import Any, Dict, Iterable, List, Optional, Tuple

import yaml


# --------------------------------------------------------------------------
# Enterprise-number lookup -- only major vendors.  Anything not here is
# bucketed under ``enterprise-<N>`` so we can still emit a YAML file.
# --------------------------------------------------------------------------


ENTERPRISE_VENDORS: Dict[str, str] = {
    "2": "ibm",
    "9": "cisco",
    "11": "hp",
    "12": "bull",
    "14": "3com",
    "20": "raytheon",
    "23": "novell",
    "25": "siemens",
    "26": "att",
    "32": "santacruz",
    "36": "dec",
    "42": "sun",
    "43": "intel",
    "45": "synoptics",
    "52": "enterasys",
    "63": "apple",
    "81": "lantronix",
    "111": "oracle",
    "116": "hitachi",
    "119": "nec",
    "166": "cabletron",
    "171": "dlink",
    "193": "ericsson",
    "207": "allied-telesis",
    "232": "compaq",
    "244": "lantronix",
    "253": "xerox",
    "259": "amazon",
    "272": "bay-networks",
    "311": "microsoft",
    "318": "apc",
    "343": "intel",
    "353": "atm-forum",
    "388": "bay-networks",
    "434": "delta-electronics",
    "476": "epson",
    "534": "eaton",
    "541": "watchguard",
    "562": "nortel",
    "631": "icomm-tech",
    "636": "fortinet",
    "664": "vipersmith",
    "674": "dell",
    "685": "raritan",
    "789": "netapp",
    "800": "purestorage",
    "847": "westell",
    "925": "level3",
    "935": "telia",
    "1466": "ibm",
    "1588": "qlogic",
    "1714": "infortrend",
    "1966": "perle",
    "1602": "canon",
    "1916": "extreme-networks",
    "1991": "foundry",
    "1996": "fortinet",
    "2011": "huawei",
    "2021": "ucd-snmp",
    "2272": "nortel",
    "2356": "ibm",
    "2434": "fluke",
    "2435": "brocade",
    "2544": "adva",
    "2467": "ericsson",
    "2620": "checkpoint",
    "2636": "juniper",
    "3052": "asentria",
    "3375": "f5",
    "3607": "cerent",
    "3808": "cyberpower",
    "3955": "linksys",
    "4115": "fluke-networks",
    "4413": "broadcom",
    "4526": "netgear",
    "4874": "redback",
    "4881": "infoblox",
    "5004": "fujitsu",
    "5263": "polycom",
    "5528": "netscreen",
    "5624": "enterasys",
    "5651": "fortinet",
    "6027": "force10",
    "6296": "dasan",
    "6486": "alcatel-lucent",
    "6527": "alcatel-lucent",
    "6876": "vmware",
    "6889": "avaya",
    "7262": "dragonwave",
    "7483": "tropic",
    "8164": "wago",
    "8691": "moxa",
    "8741": "veritas",
    "9148": "vyatta",
    "9303": "packetfront",
    "9484": "blue-coat",
    "9789": "astaro",
    "10418": "iomega",
    "10876": "amazon",
    "10942": "voltaire",
    "11129": "google",
    "11863": "aerohive",
    "12148": "fanvil",
    "12356": "fortinet",
    "12394": "alcatel",
    "12740": "netscout",
    "13335": "cloudflare",
    "13601": "ekinops",
    "14179": "airespace",
    "14525": "trapeze",
    "14823": "aruba",
    "14988": "mikrotik",
    "15004": "schneider",
    "17713": "cambium",
    "18070": "bti",
    "19746": "data-domain",
    "20916": "roomalert",
    "21239": "vertiv",
    "16108": "extricom",
    "16690": "openbsd",
    "17163": "stargate",
    "17163.": "stargate",
    "18334": "ricoh",
    "18412": "comware",
    "19046": "lexmark",
    "20294": "polycom",
    "21091": "bluecat",
    "21296": "ucd",
    "21317": "asus",
    "21796": "ridgewave",
    "22610": "midnight-network",
    "23867": "intel",
    "25053": "ruckus",
    "25281": "vyatta",
    "25461": "palo-alto",
    "25506": "h3c",
    "25623": "openvas",
    "29462": "stulz",
    "26543": "ibm",
    "27975": "ubiquiti",
    "28658": "imedia",
    "29671": "barracuda",
    "30065": "arista",
    "30464": "wd",
    "31317": "audiocodes",
    "31619": "iomega",
    "32285": "openvpn",
    "33049": "synology",
    "33452": "openstack",
    "33688": "fujitsu",
    "34297": "ssh",
    "34823": "infoblox",
    "35265": "alibaba",
    "36673": "ip-infusion",
    "35780": "viavi",
    "36632": "infinera",
    "36968": "lancom",
    "37072": "qnap",
    "37288": "facebook",
    "39729": "openvswitch",
    "40310": "azure",
    "41112": "ubiquiti",
    "41263": "nutanix",
    "41789": "amazon",
    "44641": "cumulus",
    "47272": "fanvil",
    "47351": "tencent",
    "49165": "yealink",
    "50130": "exfo",
    "50266": "ruckus",
    "53246": "comware",
    "55187": "intel-rs",
    "57625": "tplink",
}


def safe_slug(s: str) -> str:
    s = re.sub(r"[^a-zA-Z0-9._-]+", "-", s).strip("-").lower()
    return s or "unknown"


def vendor_for_oid(oid: str) -> str:
    """Return the vendor slug derived from the trap OID.

    Standard tree -> ``standard``.  IEEE 802.1AB (LLDP) tree -> ``ieee-lldp``.
    Enterprise tree -> look up the enterprise number; unknown enterprise
    numbers become ``enterprise-<N>``.  Malformed or absent OIDs become
    ``unknown``.
    """
    if not oid:
        return "unknown"
    if oid.startswith("1.3.6.1.2.1.") or oid.startswith("1.3.6.1.6.3."):
        return "standard"
    if oid.startswith("1.0.8802."):
        # IEEE 802.1AB-2005 (LLDP) and related IEEE Std MIBs.
        return "ieee-lldp"
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
    """Build the per-file deduped varbind table.

    Returns:
      table:    name -> slim_varbind dict (file-level ``varbinds:`` map)
      inline:   list of (trap_oid, vb) for varbinds we could NOT dedup because
                they conflict with an earlier entry of the same name. These
                stay inline in the trap entry.
    """
    table: Dict[str, Dict[str, Any]] = {}
    inline: List[Tuple[str, Dict[str, Any]]] = []
    for rec in recs:
        trap_oid = rec.get("oid") or ""
        for vb in rec.get("varbinds") or []:
            if not isinstance(vb, dict):
                continue
            name = vb.get("name")
            slim = slim_varbind(vb)
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
    if rec.get("name"):
        entry["name"] = rec["name"]
    entry["category"] = rec.get("category") or "unknown"
    entry["severity"] = rec.get("severity") or "info"
    if rec.get("description"):
        entry["description"] = rec["description"]
    if rec.get("mib"):
        entry["mib"] = rec["mib"]
    if rec.get("trap_status"):
        entry["status"] = rec["trap_status"]

    # Reference-list form: each varbind is just its name. Plugin resolves
    # via the file-level ``varbinds:`` table. Anonymous or conflicting
    # varbinds fall back to inline-dict shape on the trap entry.
    refs: List[Any] = []
    for vb in rec.get("varbinds") or []:
        if not isinstance(vb, dict):
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
        try:
            with open(path) as f:
                rec = json.load(f)
        except Exception as exc:
            logging.warning("skip %s: %s", path, exc)
            n_skipped += 1
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
        sample_traps = [
            {"oid": r.get("oid"), "name": r.get("name"), "mib": r.get("mib")}
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
