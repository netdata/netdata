#!/usr/bin/env python3
"""
SOW-0033 - SNMP MIB Mechanical Extraction Pipeline.

Walks all configured MIB source directories, compiles every MIB module
with pysmi, and emits one JSONL record per NOTIFICATION-TYPE / TRAP-TYPE
definition to output/extracted.jsonl.

Each record carries the trap's OID/name/MIB-meta/description plus
fully-resolved varbind metadata (type, enum, max-access, status,
description) for every referenced OBJECT-TYPE.

This stage is fully mechanical; no LLM, no human judgement.
"""

from __future__ import annotations

import argparse
import json
import logging
import os
import sys
import time
import traceback
from collections import OrderedDict
from typing import Any, Dict, Iterable, List, Optional, Tuple

from pysmi.codegen import JsonCodeGen
from pysmi.compiler import MibCompiler
from pysmi.parser import SmiV1CompatParser
from pysmi.reader import FileReader
from pysmi.searcher import StubSearcher
from pysmi.writer import CallbackWriter


# Priority-ordered search path. First match for a given MIB name wins.
# - IETF canonical first (netdisco/rfc), then pysnmp/mibs (canonical multi-
#   vendor curation tracked by the pysnmp project), then vendor-canonical
#   (cisco v2), then the broader community archives.
DEFAULT_SOURCE_DIRS: List[str] = [
    "/opt/baddisk/monitoring/repos/netdisco/netdisco-mibs/rfc",
    "/opt/baddisk/monitoring/repos/snmp/mibs/src",
    "/opt/baddisk/monitoring/repos/snmp/cisco__cisco-mibs/v2",
    "/opt/baddisk/monitoring/repos/snmp/cisco__cisco-mibs/v1",
    "/opt/baddisk/monitoring/repos/snmp/Poil__MIBs",
    "/opt/baddisk/monitoring/repos/snmp/Poil__MIBs/cisco_v2",
    "/opt/baddisk/monitoring/repos/snmp/kcsinclair__mibs",
    "/opt/baddisk/monitoring/repos/snmp/kmalinich__snmp-mibs",
    "/opt/baddisk/monitoring/repos/snmp/hsnodgrass__snmp_mib_archive",
    "/opt/baddisk/monitoring/repos/librenms/librenms/mibs",
    "/opt/baddisk/monitoring/repos/netdisco/netdisco-mibs",
]


# When walking these vendor-pack roots (netdisco-mibs/<vendor>/...,
# librenms/mibs/<vendor>/...), include every immediate subdirectory as a
# search-path entry so pysmi can resolve cross-vendor imports.
VENDOR_PACK_ROOTS: List[str] = [
    "/opt/baddisk/monitoring/repos/netdisco/netdisco-mibs",
    "/opt/baddisk/monitoring/repos/librenms/librenms/mibs",
]


# Skip files that obviously are not MIB sources.
NON_MIB_BASENAMES = {
    "readme",
    "readme.md",
    "license",
    "license.txt",
    ".gitignore",
    ".gitattributes",
    "makefile",
    "version",
}
NON_MIB_SUFFIXES = {
    ".md",
    ".rst",
    ".py",
    ".pyc",
    ".sh",
    ".pl",
    ".yaml",
    ".yml",
    ".json",
    ".html",
    ".pdf",
    ".png",
    ".jpg",
    ".jpeg",
    ".gif",
    ".pcap",
    ".log",
    ".diff",
    ".patch",
}


# Symbols/classes pysmi emits that we treat as trap-style notifications.
NOTIFICATION_CLASSES = {"notificationtype", "traptype"}


# Extra classes whose `objects` listing pysmi resolves (textual conventions,
# imports) — used for full enum / display-hint resolution.
OBJECT_CLASSES = {"objecttype"}
TC_CLASSES = {"textualconvention", "type"}


# --------------------------------------------------------------------------
# MIB discovery
# --------------------------------------------------------------------------


def looks_like_mib_filename(path: str) -> bool:
    base = os.path.basename(path).lower()
    if base.startswith("."):
        return False
    if base in NON_MIB_BASENAMES:
        return False
    _, ext = os.path.splitext(base)
    if ext in NON_MIB_SUFFIXES:
        return False
    return True


def strip_mib_extension(filename: str) -> str:
    """Derive the MIB module name from a filename.

    Vendor archives use ``.txt``, ``.mib``, ``.my``, ``.MIB``, or no
    extension at all.  Heuristic: strip a single known extension if present;
    otherwise return the basename as-is.
    """
    base = os.path.basename(filename)
    name, ext = os.path.splitext(base)
    if ext.lower() in {".txt", ".mib", ".my", ".smi", ".asn1"}:
        return name
    return base


def discover_mib_names(source_dirs: List[str]) -> Tuple[List[str], Dict[str, str]]:
    """Walk source_dirs in priority order; return unique MIB module names.

    Returns (ordered list of unique names, {name: source_file_path}).  First
    occurrence wins for any given name.
    """
    seen: "OrderedDict[str, str]" = OrderedDict()
    for root in source_dirs:
        if not os.path.isdir(root):
            continue
        for dirpath, _dirnames, filenames in os.walk(root):
            for fn in filenames:
                if not looks_like_mib_filename(fn):
                    continue
                path = os.path.join(dirpath, fn)
                name = strip_mib_extension(fn)
                # pysmi MIB module names cannot contain '.', spaces, etc.
                if not name or any(c in name for c in (" ", "\t")):
                    continue
                if name in seen:
                    continue
                seen[name] = path
    return list(seen.keys()), dict(seen)


def expand_vendor_pack_paths(source_dirs: List[str]) -> List[str]:
    """Expand source dirs with vendor-pack subdirectories.

    For roots in VENDOR_PACK_ROOTS, add each immediate subdirectory so pysmi
    can resolve imports that reference modules located inside vendor
    subdirectories (e.g., ``netdisco-mibs/cisco/CISCO-...``).
    """
    expanded: "OrderedDict[str, None]" = OrderedDict()
    for d in source_dirs:
        expanded[d] = None
    for root in VENDOR_PACK_ROOTS:
        if not os.path.isdir(root):
            continue
        for entry in sorted(os.listdir(root)):
            sub = os.path.join(root, entry)
            if os.path.isdir(sub) and not entry.startswith("."):
                expanded[sub] = None
    return list(expanded.keys())


# --------------------------------------------------------------------------
# Per-MIB compilation (one MIB at a time so a failure does not kill peers)
# --------------------------------------------------------------------------


class CompilerHarness:
    """Wrap a pysmi compiler and collect emitted JSON documents.

    A single harness is reused across the run; it accumulates a global
    symbol table keyed by ``module:object`` so cross-MIB OBJECTS references
    in NOTIFICATION-TYPE definitions can be resolved.
    """

    def __init__(self, source_dirs: List[str]):
        """Initialize the compiler and callback writer."""
        self.source_dirs = source_dirs
        self.modules: Dict[str, Dict[str, Any]] = {}
        # Track which source file produced each compiled module so the
        # extraction record can name the on-disk MIB.
        self.module_source_paths: Dict[str, str] = {}

        def _writer(mib_name: str, json_doc: str, *_args: Any, **_kwargs: Any) -> None:
            try:
                self.modules[mib_name] = json.loads(json_doc)
            except Exception as exc:
                # Malformed JSON from pysmi is exceptionally rare; record
                # nothing here and let the caller see no module came out.
                logging.debug("pysmi emitted malformed JSON for %s: %s", mib_name, exc)

        self._writer = CallbackWriter(_writer)
        self._code_gen = JsonCodeGen()
        self._parser = SmiV1CompatParser()
        self._compiler = MibCompiler(self._parser, self._code_gen, self._writer)
        readers = [FileReader(d) for d in source_dirs if os.path.isdir(d)]
        self._compiler.add_sources(*readers)
        self._compiler.add_searchers(StubSearcher(*JsonCodeGen.baseMibs))

    def compile_one(self, mib_name: str) -> Tuple[str, Optional[str]]:
        """Compile a single MIB module by name.

        Returns ``(status, reason)``.  Status is whatever pysmi reports
        (``compiled``, ``unprocessed``, ``borrowed``, ``no symbols``, ...)
        when not throwing, or ``error`` when an exception is raised.
        """
        try:
            results = self._compiler.compile(
                mib_name,
                noDeps=False,
                rebuild=True,
                genTexts=True,
            )
        except Exception as exc:
            return "error", f"{type(exc).__name__}: {exc}"
        status = results.get(mib_name, "unknown")
        # pysmi also returns dependency module statuses in the same dict;
        # we only care about the one we asked for here.
        return str(status), None


# --------------------------------------------------------------------------
# Symbol resolution
# --------------------------------------------------------------------------


def build_global_symbols(modules: Dict[str, Dict[str, Any]]) -> Dict[Tuple[str, str], Dict[str, Any]]:
    """Index every symbol by module and name.

    Cross-module OBJECTS references in NOTIFICATION-TYPE definitions can then
    be resolved.
    """
    out: Dict[Tuple[str, str], Dict[str, Any]] = {}
    for mod_name, doc in modules.items():
        if not isinstance(doc, dict):
            continue
        for sym_name, sym in doc.items():
            if isinstance(sym, dict):
                clean = demangle_pysmi_name(sym_name)
                out[(mod_name, clean)] = sym
                # Also keep the mangled alias so OBJECTS references that
                # quote the mangled form still resolve to the same record.
                if clean != sym_name:
                    out[(mod_name, sym_name)] = sym
    return out


def find_module_identity(doc: Dict[str, Any]) -> Optional[Dict[str, Any]]:
    """Return the moduleIdentity entry when present.

    The returned entry can include description, organization, contact-info,
    last-updated, and revisions.
    """
    for sym in doc.values():
        if isinstance(sym, dict) and sym.get("class") == "moduleidentity":
            return sym
    return None


def render_syntax(syntax: Any) -> Tuple[Optional[str], Optional[str], Optional[Dict[str, int]]]:
    """Render a pysmi ``syntax`` block.

    enum_map is keyed by **string** numeric value -> symbolic name (as
    required by netdata.md §7) so YAML round-trips without surprise.
    """
    if not isinstance(syntax, dict):
        return None, None, None
    t = syntax.get("type")
    constraints = syntax.get("constraints")
    constraints_str: Optional[str] = None
    enum_map: Optional[Dict[str, int]] = None
    if isinstance(constraints, dict):
        enum_obj = constraints.get("enumeration")
        if isinstance(enum_obj, dict):
            # pysmi emits {symbol: value}; invert to {str(value): symbol}.
            enum_map = {str(v): k for k, v in enum_obj.items()}
        range_obj = constraints.get("range")
        if isinstance(range_obj, list) and range_obj:
            parts = []
            for r in range_obj:
                if isinstance(r, dict):
                    parts.append(f"{r.get('min')}..{r.get('max')}")
            if parts:
                constraints_str = f"({'|'.join(parts)})"
        size_obj = constraints.get("size")
        if isinstance(size_obj, list) and size_obj:
            parts = []
            for r in size_obj:
                if isinstance(r, dict):
                    parts.append(f"{r.get('min')}..{r.get('max')}")
            if parts and not constraints_str:
                constraints_str = f"SIZE({'|'.join(parts)})"
        bits_obj = constraints.get("bits")
        if isinstance(bits_obj, dict) and not enum_map:
            enum_map = {str(v): k for k, v in bits_obj.items()}
    return (t, constraints_str, enum_map)


# pysmi prefixes Python keywords with ``_pysmi_`` so its generated code is
# syntactically valid Python. The mangled name is NOT the canonical SMI
# symbol -- snmptranslate / snmptrapd / MIB browsers all use the original.
# We strip the prefix everywhere we surface a name so the shipped pack
# matches what real tools produce.
_PYSMI_KEYWORD_PREFIX = "_pysmi_"


def demangle_pysmi_name(name: Optional[str]) -> Optional[str]:
    if isinstance(name, str) and name.startswith(_PYSMI_KEYWORD_PREFIX):
        return name[len(_PYSMI_KEYWORD_PREFIX):]
    return name


def resolve_varbind(
    obj_ref: Any,
    symbols: Dict[Tuple[str, str], Dict[str, Any]],
) -> Dict[str, Any]:
    """Resolve one NOTIFICATION-TYPE OBJECTS entry to a varbind record.

    Falls back to the raw reference if the target symbol cannot be found in the
    global symbol table.
    """
    if isinstance(obj_ref, dict):
        ref_mod = obj_ref.get("module")
        ref_name = obj_ref.get("object")
    else:
        ref_mod = None
        ref_name = obj_ref if isinstance(obj_ref, str) else None

    # Strip pysmi's Python-keyword prefix from BOTH the ref-name and the
    # surfaced record name so downstream tooling sees the canonical SMI
    # symbol.
    ref_name_clean = demangle_pysmi_name(ref_name)

    rec: Dict[str, Any] = {
        "name": ref_name_clean,
        "module": ref_mod,
        "resolved": False,
    }
    ref_name = ref_name_clean
    if not ref_name:
        return rec

    sym: Optional[Dict[str, Any]] = None
    if ref_mod is not None:
        sym = symbols.get((ref_mod, ref_name))
    if sym is None:
        # Module qualifier may be missing or wrong; try every module.
        for (m, n), s in symbols.items():
            if n == ref_name and s.get("class") in OBJECT_CLASSES:
                sym = s
                if not ref_mod:
                    rec["module"] = m
                break

    if sym is None:
        return rec

    rec["resolved"] = True
    rec["oid"] = sym.get("oid")
    rec["max_access"] = sym.get("maxaccess")
    rec["status"] = sym.get("status")
    rec["description"] = sym.get("description")
    rec["reference"] = sym.get("reference")
    rec["nodetype"] = sym.get("nodetype")

    t, constraints, enum_map = render_syntax(sym.get("syntax"))
    if t is not None:
        rec["syntax"] = t
    if constraints is not None:
        rec["syntax_constraints"] = constraints
    if enum_map is not None:
        rec["enum"] = enum_map
    # Surface the TC name when the syntax referenced one.
    syntax = sym.get("syntax")
    if isinstance(syntax, dict):
        tcb = syntax.get("tcbase")
        if isinstance(tcb, str):
            rec["tc"] = tcb
    return rec


# --------------------------------------------------------------------------
# Main extraction loop
# --------------------------------------------------------------------------


def extract_traps_from_module(
    mib_name: str,
    doc: Dict[str, Any],
    symbols: Dict[Tuple[str, str], Dict[str, Any]],
    source_path: Optional[str],
) -> Iterable[Dict[str, Any]]:
    identity = find_module_identity(doc) or {}
    mib_meta = {
        "mib_description": identity.get("description"),
        "mib_organization": identity.get("organization"),
        "mib_contact": identity.get("contactinfo"),
        "mib_last_updated": identity.get("lastupdated"),
        "mib_revisions": identity.get("revisions") or [],
    }
    for sym_name, sym in doc.items():
        if not isinstance(sym, dict):
            continue
        if sym.get("class") not in NOTIFICATION_CLASSES:
            continue
        objects = sym.get("objects") or []
        varbinds = [resolve_varbind(o, symbols) for o in objects]
        yield {
            "oid": sym.get("oid"),
            "name": demangle_pysmi_name(sym_name),
            "class": sym.get("class"),
            "mib": mib_name,
            **mib_meta,
            "trap_description": sym.get("description"),
            "trap_reference": sym.get("reference"),
            "trap_status": sym.get("status"),
            "varbinds": varbinds,
            "source_mib_file": source_path,
        }


def stratify_log(message: str, level: int = logging.INFO) -> None:
    logging.log(level, message)


def build_arg_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="Extract SNMP trap metadata from a MIB corpus.",
    )
    parser.add_argument(
        "--mibs",
        nargs="*",
        default=None,
        help="Restrict to this list of MIB module names (used by tests).",
    )
    parser.add_argument(
        "--all",
        action="store_true",
        help="Process every MIB found in the configured source directories.",
    )
    parser.add_argument(
        "--limit",
        type=int,
        default=0,
        help="Stop after compiling this many MIBs (0 = unlimited).",
    )
    parser.add_argument(
        "--resume",
        action="store_true",
        help="Skip MIBs already represented in output/extracted.jsonl.",
    )
    parser.add_argument(
        "--out-dir",
        default=os.path.join(os.path.dirname(os.path.abspath(__file__)), "output"),
        help="Output directory (default: ./output).",
    )
    parser.add_argument(
        "--log-level",
        default="WARNING",
        help="Python logging level (DEBUG, INFO, WARNING, ERROR).",
    )
    parser.add_argument(
        "--progress-every",
        type=int,
        default=100,
        help="Log progress every N MIBs (0 disables).",
    )
    return parser


def select_mib_names(
    args: argparse.Namespace,
    mib_names_in_priority: List[str],
    name_to_path: Dict[str, str],
) -> Optional[List[str]]:
    if args.mibs and not args.all:
        target = [m for m in args.mibs if m in name_to_path]
        missing = sorted(set(args.mibs) - set(target))
        if missing:
            stratify_log(f"Requested MIBs not found in corpus: {missing}", logging.WARNING)
        return target or list(args.mibs)
    if args.all:
        return mib_names_in_priority
    stratify_log("Specify --mibs <NAMES> or --all.", logging.ERROR)
    return None


def load_already_done(args: argparse.Namespace, out_jsonl: str) -> set:
    already_done: set = set()
    if not args.resume or not os.path.exists(out_jsonl):
        return already_done
    with open(out_jsonl) as f:
        for line in f:
            rec = None
            try:
                rec = json.loads(line)
            except Exception as exc:
                logging.debug("skipping malformed resume record: %s", exc)
            if rec is not None and "mib" in rec:
                already_done.add(rec["mib"])
    stratify_log(f"Resuming; {len(already_done)} MIBs already in output.")
    return already_done


def compile_requested_mibs(
    args: argparse.Namespace,
    mib_names: List[str],
    already_done: set,
    harness: CompilerHarness,
    name_to_path: Dict[str, str],
) -> Tuple[Dict[str, Any], Dict[str, str], float]:
    failures: Dict[str, Any] = {}
    statuses: Dict[str, str] = {}
    t0 = time.time()
    for i, name in enumerate(mib_names, 1):
        if name in already_done:
            statuses[name] = "skipped-resume"
            continue
        status, reason = harness.compile_one(name)
        statuses[name] = status
        if reason is not None or name not in harness.modules:
            failures[name] = {
                "status": status,
                "reason": reason or f"no JSON output (status={status})",
                "source_path": name_to_path.get(name),
            }
        if args.progress_every and i % args.progress_every == 0:
            dt = time.time() - t0
            stratify_log(
                f"compile progress {i}/{len(mib_names)} compiled={len(harness.modules)} "
                f"failures={len(failures)} elapsed={dt:.1f}s",
            )

    dt_compile = time.time() - t0
    stratify_log(f"Compilation finished in {dt_compile:.1f}s "
                 f"compiled={len(harness.modules)} failures={len(failures)}")
    return failures, statuses, dt_compile


def write_extracted_traps(
    args: argparse.Namespace,
    out_jsonl: str,
    mib_names: List[str],
    already_done: set,
    harness: CompilerHarness,
    symbols: Dict[Tuple[str, str], Dict[str, Any]],
    name_to_path: Dict[str, str],
) -> Tuple[Dict[str, List[Dict[str, str]]], int, int]:
    conflicts: Dict[str, List[Dict[str, str]]] = {}
    oid_first_seen: Dict[str, Tuple[str, str]] = {}
    write_mode = "a" if (args.resume and os.path.exists(out_jsonl)) else "w"
    n_records = 0
    n_varbinds_total = 0
    with open(out_jsonl, write_mode) as fout:
        for mib_name in mib_names:
            doc = harness.modules.get(mib_name)
            if not isinstance(doc, dict) or mib_name in already_done:
                continue
            src = name_to_path.get(mib_name)
            for rec in extract_traps_from_module(mib_name, doc, symbols, src):
                oid = rec.get("oid")
                if oid:
                    if oid in oid_first_seen:
                        prev_mib, prev_name = oid_first_seen[oid]
                        # Keep only the first occurrence; log subsequent
                        # collisions for review.
                        conflicts.setdefault(oid, []).append(
                            {"mib": mib_name, "name": rec.get("name") or ""},
                        )
                        if not conflicts[oid][0].get("first"):
                            conflicts[oid].insert(0, {
                                "first": True,
                                "mib": prev_mib,
                                "name": prev_name,
                            })
                        # Skip writing the duplicate so downstream tooling
                        # sees one trap per OID; the conflict file records
                        # all collisions for auditability.
                        continue
                    oid_first_seen[oid] = (mib_name, rec.get("name") or "")
                fout.write(json.dumps(rec, separators=(",", ":")) + "\n")
                n_records += 1
                n_varbinds_total += len(rec.get("varbinds") or [])
    return conflicts, n_records, n_varbinds_total


def write_json_file(path: str, data: Any) -> None:
    with open(path, "w") as f:
        json.dump(data, f, indent=2, sort_keys=True)


def build_report(
    sources_with_packs: List[str],
    mib_names_in_priority: List[str],
    mib_names: List[str],
    harness: CompilerHarness,
    failures: Dict[str, Any],
    statuses: Dict[str, str],
    n_records: int,
    n_varbinds_total: int,
    conflicts: Dict[str, List[Dict[str, str]]],
    dt_compile: float,
    out_jsonl: str,
) -> Dict[str, Any]:
    return {
        "source_dirs": sources_with_packs,
        "mibs_discovered": len(mib_names_in_priority),
        "mibs_requested": len(mib_names),
        "mibs_compiled_ok": len(harness.modules),
        "mibs_failed": len(failures),
        "mibs_resumed_skip": sum(1 for s in statuses.values() if s == "skipped-resume"),
        "traps_extracted": n_records,
        "varbinds_total": n_varbinds_total,
        "oid_conflicts": len(conflicts),
        "elapsed_compile_seconds": dt_compile,
        "out_jsonl": out_jsonl,
    }


def main() -> int:
    parser = build_arg_parser()
    args = parser.parse_args()

    logging.basicConfig(
        level=getattr(logging, args.log_level.upper(), logging.WARNING),
        format="%(asctime)s %(levelname)s %(message)s",
    )

    os.makedirs(args.out_dir, exist_ok=True)
    out_jsonl = os.path.join(args.out_dir, "extracted.jsonl")
    out_failed = os.path.join(args.out_dir, "failed-mibs.json")
    out_conflicts = os.path.join(args.out_dir, "dedup-conflicts.json")
    out_report = os.path.join(args.out_dir, "extraction-report.json")

    # Discover MIBs.
    sources_with_packs = expand_vendor_pack_paths(DEFAULT_SOURCE_DIRS)
    mib_names_in_priority, name_to_path = discover_mib_names(sources_with_packs)
    stratify_log(
        f"Discovered {len(mib_names_in_priority)} unique MIB module names across "
        f"{len(sources_with_packs)} source directories.",
    )

    mib_names = select_mib_names(args, mib_names_in_priority, name_to_path)
    if mib_names is None:
        return 2

    if args.limit:
        mib_names = mib_names[: args.limit]

    already_done = load_already_done(args, out_jsonl)

    harness = CompilerHarness(sources_with_packs)

    # Compile every requested MIB; failures land in failed-mibs.json.
    failures, statuses, dt_compile = compile_requested_mibs(
        args,
        mib_names,
        already_done,
        harness,
        name_to_path,
    )

    # Resolve varbinds across modules.
    symbols = build_global_symbols(harness.modules)

    # Emit per-trap JSONL.
    conflicts, n_records, n_varbinds_total = write_extracted_traps(
        args,
        out_jsonl,
        mib_names,
        already_done,
        harness,
        symbols,
        name_to_path,
    )

    # Write side reports.
    write_json_file(out_failed, failures)
    write_json_file(out_conflicts, conflicts)
    report = build_report(
        sources_with_packs,
        mib_names_in_priority,
        mib_names,
        harness,
        failures,
        statuses,
        n_records,
        n_varbinds_total,
        conflicts,
        dt_compile,
        out_jsonl,
    )
    write_json_file(out_report, report)

    stratify_log(
        f"Wrote {n_records} trap records ({n_varbinds_total} varbinds total) "
        f"to {out_jsonl}; conflicts={len(conflicts)} failures={len(failures)}.",
    )
    print(json.dumps(report, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        print("Interrupted.", file=sys.stderr)
        sys.exit(130)
    except Exception:
        traceback.print_exc()
        sys.exit(1)
