#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later
#
# netdata-sos - collect a sanitized diagnostic bundle for Netdata support tickets.
#
# What is collected and WHY each item is included is documented in
# packaging/installer/SUPPORT-BUNDLE.md - read it before adding or changing
# a collection item, and keep it in sync with this script.
#
# Design contract:
#   - Works with or without a running agent, with or without root (degrades gracefully).
#   - Zero system impact: idle CPU/IO priority, per-command timeouts, global deadline,
#     size caps, strictly read-only outside its own staging dir.
#   - Secrets are ALWAYS redacted (non-optional). PII (IPs, MACs, emails, hostnames)
#     pseudonymized by default; --no-obfuscate disables PII pass only.
#   - Bundle is legible to humans AND AI agents: triage-ordered directories, pristine
#     file copies, provenance in MANIFEST.json, human summary.txt, README.md inside.
#
# Usage: sudo sh netdata-sos.sh [options]
#   -o, --output DIR     where to write the tarball (default: /tmp)
#   --since HOURS        log window in hours (default: 24)
#   --timeout SECONDS    per-command timeout (default: 10)
#   --no-obfuscate       disable PII pseudonymization (secrets STILL redacted)
#   --keep-staging       keep staging dir for inspection
#   -v, --version        print version
#   -h, --help           this help

set -u

# --- self-demotion FIRST (before arg parsing consumes "$@"): never compete
# --- with real workloads
if [ -z "${ND_SOS_DEMOTED:-}" ]; then
  ND_SOS_DEMOTED=1; export ND_SOS_DEMOTED
  if command -v ionice >/dev/null 2>&1; then
    exec nice -n 19 ionice -c 3 sh "$0" "$@"
  fi
  exec nice -n 19 sh "$0" "$@"
fi

VERSION="1.0.0"
OUTDIR="/tmp"
SINCE_HOURS=24
CMD_TIMEOUT=10
OBFUSCATE=1
KEEP_STAGING=0
GLOBAL_DEADLINE=240
LOG_CAP=5242880          # 5 MiB per log file
FILE_CAP=1048576         # 1 MiB per config/state file
API_CAP=2097152          # 2 MiB per API response

while [ $# -gt 0 ]; do
  case "$1" in
    -o|--output) OUTDIR="$2"; shift 2 ;;
    --since) SINCE_HOURS="$2"; shift 2 ;;
    --timeout) CMD_TIMEOUT="$2"; shift 2 ;;
    --no-obfuscate) OBFUSCATE=0; shift ;;
    --keep-staging) KEEP_STAGING=1; shift ;;
    -v|--version) echo "netdata-sos $VERSION"; exit 0 ;;
    -h|--help) sed -n '4,27p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown option: $1" >&2; exit 1 ;;
  esac
done

case "$SINCE_HOURS" in *[!0-9]*|'') echo "--since must be a positive integer (hours)" >&2; exit 1 ;; esac
case "$CMD_TIMEOUT" in *[!0-9]*|'') echo "--timeout must be a positive integer (seconds)" >&2; exit 1 ;; esac

umask 077
START_TS=$(date +%s)
NOW=$(date -u +%Y%m%d-%H%M%S)
STAGING=$(mktemp -d "${TMPDIR:-/tmp}/netdata-sos.XXXXXX") || exit 1
BUNDLE="netdata-sos-${NOW}-$$"
WORK="$STAGING/$BUNDLE"
mkdir -p "$WORK"
MAP_FILE="$STAGING/map.tsv"; : > "$MAP_FILE"
MANIFEST_ROWS="$STAGING/manifest.rows"; : > "$MANIFEST_ROWS"
ERRORS="$STAGING/errors.txt"; : > "$ERRORS"

cleanup() { [ "$KEEP_STAGING" = "1" ] || rm -rf "$STAGING"; }
trap cleanup EXIT INT TERM

info() { printf ' [*] %s\n' "$*" >&2; }
now_s() { date +%s; }
deadline_exceeded() { [ $(( $(now_s) - START_TS )) -ge "$GLOBAL_DEADLINE" ]; }

# timeout capability: GNU/FreeBSD support -k (kill-after); busybox does not; macOS has none
have_timeout=0
if command -v timeout >/dev/null 2>&1; then
  if timeout -k 2 5 true 2>/dev/null; then have_timeout=2
  elif timeout 5 true 2>/dev/null; then have_timeout=1
  fi
fi
run_capped() { # run_capped <seconds> <cmd...>
  _t="$1"; shift
  case "$have_timeout" in
    2) timeout -k 2 "$_t" "$@" ;;
    1) timeout "$_t" "$@" ;;
    *) "$@" ;;
  esac
}

# --- sanitizer: single awk pass, portable (gawk/mawk/busybox) ---------------
# pass 1 (always): credential-key values, URL/DSN creds, JWTs, private keys
# pass 2 (default): emails, MACs, IPv4 pseudonyms (stable via map), hostnames
HOST_SHORT=$(hostname 2>/dev/null || echo "")
HOST_FQDN=$(hostname -f 2>/dev/null || echo "")
[ "$HOST_SHORT" = "localhost" ] && HOST_SHORT=""
[ "$HOST_FQDN" = "localhost" ] && HOST_FQDN=""
[ ${#HOST_SHORT} -lt 4 ] && HOST_SHORT=""
# the invoking user's name is PII too (ps USER column, /home/<name> paths)
RUN_USER=$(id -un 2>/dev/null || echo "")
case "$RUN_USER" in root|netdata|"") RUN_USER="" ;; esac
[ ${#RUN_USER} -lt 3 ] && RUN_USER=""

sanitize_file() {
  _f="$1"
  [ -f "$_f" ] || return 0
  if awk -v obf="$OBFUSCATE" -v mapfile="$MAP_FILE" \
      -v host_short="$HOST_SHORT" -v host_fqdn="$HOST_FQDN" -v run_user="$RUN_USER" '
  BEGIN {
    nsec = split("api key,apikey,token,password,passwd,secret,community,bearer,webhook,license key,auth,credential,cookie,passphrase,proxy user,proxy pass,username,dsn,private key,access key,session,recipient,account sid,priv key", SK, ",");
    for (i = 1; i <= nsec; i++) gsub(/[-_]/, " ", SK[i]);
    nip = 0;
    while ((getline line < mapfile) > 0) {
      split(line, a, "\t");
      if (a[1] == "ip") { ipmap[a[2]] = a[3]; nip++; }
      else if (a[1] == "host") hostmap[a[2]] = a[3];
      else if (a[1] == "user") usermap[a[2]] = a[3];
      else if (a[1] == "ip6") { ip6map[a[2]] = a[3]; nip6++; }
      else if (a[1] == "fqdn") { fqmap[a[2]] = a[3]; nfq++; }
    }
    close(mapfile);
  }
  function harmless_value(v) {
    # boolean/mode literals and absolute paths are never secrets; keeping them
    # preserves diagnostics like "bearer token protection = no" or
    # "netdata management api key file = /var/lib/..."
    gsub(/^[ \t]+|[ \t]+$/, "", v);
    if (v ~ /^(yes|no|true|false|on|off|auto|none|enabled|disabled)$/) return 1;
    if (v ~ /^\//) return 1;
    return 0;
  }
  function redact_kv(line,   i, k, pos, posc, pose, key, lk) {
    # ini/yaml/env style: <key> = value | <key>: value | KEY=value
    # (JSON-shaped lines are owned by redact_json, which preserves quoting)
    if (line ~ /^[ \t]*"/) return line;
    pose = index(line, "="); posc = index(line, ":");
    pos = 0;
    if (pose > 0 && (posc == 0 || pose < posc)) pos = pose;
    else if (posc > 0) pos = posc;
    if (pos > 1) {
      key = substr(line, 1, pos - 1);
      gsub(/^[ \t#]+|[ \t]+$/, "", key);
      # only plausible config keys: short, no sentence/shell punctuation
      # (prevents prose like "token and ... not collected: X" matching as a key)
      if (length(key) > 64 || key ~ /["`;|()]/) return line;
      lk = tolower(key);
      gsub(/[-_]/, " ", lk);
      for (i = 1; i <= nsec; i++) {
        if (index(lk, SK[i]) > 0 && length(substr(line, pos + 1)) > 1) {
          if (harmless_value(substr(line, pos + 1))) return line;
          return substr(line, 1, pos) " [REDACTED]";
        }
      }
    }
    return line;
  }
  function redact_json(line,   out, rest, key, lk, i, k, m, v) {
    # "key": "value" pairs, possibly many per line
    out = ""; rest = line;
    while (match(rest, /"[^"]+"[ \t]*:[ \t]*"[^"]*"/)) {
      m = substr(rest, RSTART, RLENGTH);
      key = m; sub(/^"/, "", key); sub(/".*/, "", key);
      v = m; sub(/^"[^"]+"[ \t]*:[ \t]*"/, "", v); sub(/"$/, "", v);
      lk = tolower(key); gsub(/[-_]/, " ", lk);
      for (i = 1; i <= nsec; i++) {
        if (index(lk, SK[i]) > 0 && !harmless_value(v)) {
          sub(/:[ \t]*"[^"]*"/, ": \"[REDACTED]\"", m);
          break;
        }
      }
      out = out substr(rest, 1, RSTART - 1) m;
      rest = substr(rest, RSTART + RLENGTH);
    }
    return out rest;
  }
  function pseudo_ip(ip,   p) {
    if (ip ~ /^127\./ || ip == "0.0.0.0" || ip ~ /^255\./) return ip;
    if (!(ip in ipmap)) {
      nip++; ipmap[ip] = "ip-" nip;
      print "ip\t" ip "\t" ipmap[ip] >> mapfile;
    }
    return ipmap[ip];
  }
  function replace_ips(line,   out, rest, ip) {
    out = ""; rest = line;
    while (match(rest, /[0-9][0-9]?[0-9]?\.[0-9][0-9]?[0-9]?\.[0-9][0-9]?[0-9]?\.[0-9][0-9]?[0-9]?/)) {
      ip = substr(rest, RSTART, RLENGTH);
      out = out substr(rest, 1, RSTART - 1) pseudo_ip(ip);
      rest = substr(rest, RSTART + RLENGTH);
    }
    return out rest;
  }
  function replace_ip6(line,   out, rest, cand, pre, nc, t) {
    # IPv6 pseudonymization. Candidates are hex-and-colon runs; validated to
    # avoid timestamps (13:38:34), file:line refs and C++ :: tokens:
    #   - not preceded by a word char, >=5 chars, contains ":"
    #   - >=3 colons or a "::" compression
    #   - contains a hex letter or a "::" (all-digit uncompressed v6 is skipped)
    # ::1 and :: are kept (loopback/wildcard).
    out = ""; rest = line;
    while (match(rest, /[0-9A-Fa-f:]+/)) {
      cand = substr(rest, RSTART, RLENGTH);
      pre = (RSTART > 1) ? substr(rest, RSTART - 1, 1) : "";
      t = cand; nc = gsub(/:/, ":", t);
      if (index(cand, ":") == 0 || pre ~ /[A-Za-z0-9._-]/ || length(cand) < 5 ||
          (nc < 3 && index(cand, "::") == 0) ||
          (cand !~ /[A-Fa-f]/ && index(cand, "::") == 0) ||
          cand == "::1") {
        out = out substr(rest, 1, RSTART + RLENGTH - 1);
        rest = substr(rest, RSTART + RLENGTH);
        continue;
      }
      if (!(cand in ip6map)) {
        nip6++; ip6map[cand] = "ip6-" nip6;
        print "ip6\t" cand "\t" ip6map[cand] >> mapfile;
      }
      out = out substr(rest, 1, RSTART - 1) ip6map[cand];
      rest = substr(rest, RSTART + RLENGTH);
    }
    return out rest;
  }
  function replace_private_fqdns(line,   out, rest, fq, nxt) {
    # hostnames under clearly-private TLDs are user infrastructure -> pseudonymize
    out = ""; rest = line;
    while (match(rest, /[A-Za-z0-9][A-Za-z0-9.-]*\.(internal|local|lan|corp|intranet|localdomain)/)) {
      fq = substr(rest, RSTART, RLENGTH);
      nxt = substr(rest, RSTART + RLENGTH, 1);
      if (nxt ~ /[A-Za-z0-9-]/) { # partial word (e.g. .locale) - keep as is
        out = out substr(rest, 1, RSTART + RLENGTH - 1);
        rest = substr(rest, RSTART + RLENGTH);
        continue;
      }
      if (!(fq in fqmap)) {
        nfq++; fqmap[fq] = "private-host-" nfq;
        print "fqdn\t" fq "\t" fqmap[fq] >> mapfile;
      }
      out = out substr(rest, 1, RSTART - 1) fqmap[fq];
      rest = substr(rest, RSTART + RLENGTH);
    }
    return out rest;
  }
  function replace_host(line, h,   p, out, idx) {
    if (h == "") return line;
    if (!(h in hostmap)) {
      hostmap[h] = "redacted-host";
      print "host\t" h "\t" hostmap[h] >> mapfile;
    }
    p = hostmap[h]; out = "";
    while ((idx = index(line, h)) > 0) {
      out = out substr(line, 1, idx - 1) p;
      line = substr(line, idx + length(h));
    }
    return out line;
  }
  function replace_user(line, u,   out, idx) {
    if (u == "") return line;
    if (!(u in usermap)) {
      usermap[u] = "redacted-user";
      print "user\t" u "\t" usermap[u] >> mapfile;
    }
    out = "";
    while ((idx = index(line, u)) > 0) {
      out = out substr(line, 1, idx - 1) usermap[u];
      line = substr(line, idx + length(u));
    }
    return out line;
  }
  {
    # NOTE: no {n,m} regex intervals anywhere in this program - older BSD awks
    # treat them as literal braces, which would silently disable redaction.
    line = $0;
    # stream.conf parent side: [<API_KEY>] / [<MACHINE_GUID>] section headers ARE secrets
    if (line ~ /^[ \t]*\[[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]-[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]-[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]-[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]-[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]\][ \t]*$/) {
      print "[REDACTED-KEY-SECTION]"; next;
    }
    line = redact_kv(line);
    if (line ~ /"[^"]+"[ \t]*:[ \t]*"/) line = redact_json(line);
    # URL creds: scheme://user:pass@  and Go DSN: user:pass@tcp(
    while (match(line, /:\/\/[^:\/@ \t]+:[^@ \t]+@/))
      line = substr(line, 1, RSTART - 1) "://[REDACTED]@" substr(line, RSTART + RLENGTH);
    while (match(line, /[A-Za-z0-9_]+:[^@ \t]+@(tcp|unix)\(/))
      line = substr(line, 1, RSTART - 1) "[REDACTED]@" substr(line, RSTART + RLENGTH - 4);
    # JWTs (the eyJ prefix is base64 for double-quote-brace)
    while (match(line, /eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+/))
      line = substr(line, 1, RSTART - 1) "[REDACTED-JWT]" substr(line, RSTART + RLENGTH);
    # HTTP auth header values anywhere in a line (headers in configs, curl -v, logs).
    # The value must contain a digit: real bearer tokens do, English words after
    # "bearer" in config prose ("bearer token protection = no") do not.
    while (match(line, /[Bb]earer[ \t]+[A-Za-z._~+\/=-]*[0-9][A-Za-z0-9._~+\/=-]*/))
      line = substr(line, 1, RSTART - 1) "Bearer [REDACTED]" substr(line, RSTART + RLENGTH);
    while (match(line, /[Bb]asic[ \t]+[A-Za-z0-9+\/=][A-Za-z0-9+\/=][A-Za-z0-9+\/=][A-Za-z0-9+\/=][A-Za-z0-9+\/=][A-Za-z0-9+\/=][A-Za-z0-9+\/=][A-Za-z0-9+\/=]+/))
      line = substr(line, 1, RSTART - 1) "Basic [REDACTED]" substr(line, RSTART + RLENGTH);
    # secrets passed as URL query parameters (access.log request lines etc.)
    out = ""; rest = line;
    while (match(rest, /[?&](token|apikey|api_key|password|passwd|secret|bearer|claim_token|claim_rooms|key|auth)=/)) {
      out = out substr(rest, 1, RSTART + RLENGTH - 1) "[REDACTED]";
      rest = substr(rest, RSTART + RLENGTH);
      sub(/^[^&" \t]+/, "", rest);
    }
    line = out rest;
    # argv/env-style secrets mid-line (ps output, command lines: -token=X, CLAIM_TOKEN=X)
    out = ""; rest = line;
    while (match(rest, /[A-Za-z0-9_.-]*(token|TOKEN|Token|password|PASSWORD|Password|passwd|PASSWD|secret|SECRET|Secret|apikey|APIKEY|ApiKey|api_key|API_KEY|community|COMMUNITY|bearer|BEARER)[ ]?=[ ]?[^&" \t[]+/)) {
      m = substr(rest, RSTART, RLENGTH);
      v = m; sub(/^[^=]*=[ ]?/, "", v);
      sub(/[ ]?=.*/, "", m);
      if (harmless_value(v))
        out = out substr(rest, 1, RSTART + RLENGTH - 1);
      else
        out = out substr(rest, 1, RSTART - 1) m "=[REDACTED]";
      rest = substr(rest, RSTART + RLENGTH);
    }
    line = out rest;
    # two-word secret keys mid-line ("api key = X" inside a captured command line)
    out = ""; rest = line;
    while (match(rest, /([Aa][Pp][Ii]|[Ll][Ii][Cc][Ee][Nn][Ss][Ee]|[Aa][Uu][Tt][Hh]|[Aa][Cc][Cc][Ee][Ss][Ss])[ ][Kk][Ee][Yy][ ]?=[ ]?[^&" \t[]+|[Pp][Rr][Oo][Xx][Yy][ ]([Uu][Ss][Ee][Rr]|[Pp][Aa][Ss][Ss]([Ww][Oo][Rr][Dd])?)[ ]?=[ ]?[^&" \t[]+/)) {
      m = substr(rest, RSTART, RLENGTH);
      v = m; sub(/^[^=]*=[ ]?/, "", v);
      sub(/[ ]?=.*/, "", m);
      if (harmless_value(v))
        out = out substr(rest, 1, RSTART + RLENGTH - 1);
      else
        out = out substr(rest, 1, RSTART - 1) m " = [REDACTED]";
      rest = substr(rest, RSTART + RLENGTH);
    }
    line = out rest;
    if (line ~ /-----BEGIN .*PRIVATE KEY/) line = "[REDACTED PRIVATE KEY]";
    if (obf == 1) {
      # emails
      while (match(line, /[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z][A-Za-z]+/))
        line = substr(line, 1, RSTART - 1) "[EMAIL]" substr(line, RSTART + RLENGTH);
      # MACs
      while (match(line, /[0-9A-Fa-f][0-9A-Fa-f]:[0-9A-Fa-f][0-9A-Fa-f]:[0-9A-Fa-f][0-9A-Fa-f]:[0-9A-Fa-f][0-9A-Fa-f]:[0-9A-Fa-f][0-9A-Fa-f]:[0-9A-Fa-f][0-9A-Fa-f]/))
        line = substr(line, 1, RSTART - 1) "[MAC]" substr(line, RSTART + RLENGTH);
      line = replace_ips(line);
      line = replace_ip6(line);
      line = replace_private_fqdns(line);
      if (host_fqdn != "") line = replace_host(line, host_fqdn);
      if (host_short != "") line = replace_host(line, host_short);
      if (run_user != "") line = replace_user(line, run_user);
    }
    print line;
  }' "$_f" > "$_f.san" 2>>"$ERRORS"; then
    mv "$_f.san" "$_f"
  else
    # fail CLOSED: never ship content the sanitizer could not process
    rm -f "$_f.san"
    echo "[netdata-sos] sanitization failed for this file - content withheld for safety" > "$_f"
  fi
}

# --- manifest ----------------------------------------------------------------
# manifest_add <rel-path> <kind:cmd|file|api> <origin> <title>
json_str() { # collapse newlines/tabs, escape backslashes and quotes
  printf '%s' "$1" | tr '\n\t' '  ' | sed 's/\\/\\\\/g; s/"/\\"/g'
}
manifest_add() {
  _rel="$1"; _kind="$2"; _origin="$3"; _title="$4"
  _bytes=0; [ -f "$WORK/$_rel" ] && _bytes=$(wc -c < "$WORK/$_rel" | tr -d ' ')
  printf '{"path":"%s","kind":"%s","origin":"%s","title":"%s","bytes":%s,"pii_obfuscated":%s}\n' \
    "$_rel" "$_kind" "$(json_str "$_origin")" "$(json_str "$_title")" "$_bytes" \
    "$([ "$OBFUSCATE" = "1" ] && echo true || echo false)" >> "$MANIFEST_ROWS"
}

# --- collectors ---------------------------------------------------------------
# collect_cmd <rel-path> <title> <cmd...>
collect_cmd() {
  _rel="$1"; _title="$2"; shift 2
  _out="$WORK/$_rel"
  if deadline_exceeded; then echo "SKIPPED: global deadline reached" > "$_out"; return 0; fi
  mkdir -p "$(dirname "$_out")"
  _t0=$(now_s)
  # header must stay a single line even for multi-line sh -c scripts
  _cmdline=$(printf '%s' "$*" | tr -d '\\' | tr '\n\t' '  ' | tr -s ' ')
  {
    printf '# netdata-sos v%s | command: %s | captured: %s\n' "$VERSION" "$_cmdline" "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    run_capped "$CMD_TIMEOUT" "$@" 2>&1
    printf '# exit: %s | duration: %ss\n' "$?" "$(( $(now_s) - _t0 ))"
  } > "$_out" 2>&1
  sanitize_file "$_out"
  manifest_add "$_rel" "cmd" "$_cmdline" "$_title"
}

# collect_file <rel-path> <title> <source-file> [cap-bytes]
collect_file() {
  _rel="$1"; _title="$2"; _src="$3"; _cap="${4:-$FILE_CAP}"
  deadline_exceeded && return 0
  [ -f "$_src" ] && [ -r "$_src" ] || return 0
  _out="$WORK/$_rel"; mkdir -p "$(dirname "$_out")"
  _size=$(wc -c < "$_src" 2>/dev/null | tr -d ' ')
  if [ "${_size:-0}" -gt "$_cap" ]; then
    tail -c "$_cap" "$_src" > "$_out" 2>>"$ERRORS"
    _origin="$_src (last $_cap of $_size bytes)"
  else
    cat "$_src" > "$_out" 2>>"$ERRORS"
    _origin="$_src"
  fi
  sanitize_file "$_out"
  manifest_add "$_rel" "file" "$_origin" "$_title"
}

# collect_cmd_raw <rel-path> <title> <cmd...> - like collect_cmd but with NO
# provenance header/trailer, for commands whose output must stay parseable
# (JSON). Provenance lives in MANIFEST.json. Removed if the command yields nothing.
collect_cmd_raw() {
  _rel="$1"; _title="$2"; shift 2
  _out="$WORK/$_rel"
  deadline_exceeded && return 0
  mkdir -p "$(dirname "$_out")"
  _cmdline=$(printf '%s' "$*" | tr -d '\\' | tr '\n\t' '  ' | tr -s ' ')
  run_capped "$CMD_TIMEOUT" "$@" > "$_out" 2>>"$ERRORS"
  if [ -s "$_out" ]; then
    sanitize_file "$_out"
    manifest_add "$_rel" "cmd" "$_cmdline" "$_title"
  else
    rm -f "$_out"
  fi
}

# collect_api <rel-path> <title> <url-path>
NDPORT=19999
api_ok=0
collect_api() {
  _rel="$1"; _title="$2"; _url="http://127.0.0.1:${NDPORT}$3"
  deadline_exceeded && return 0
  _out="$WORK/$_rel"; mkdir -p "$(dirname "$_out")"
  if command -v curl >/dev/null 2>&1; then
    curl -sf --max-time "$CMD_TIMEOUT" "$_url" 2>>"$ERRORS" | head -c "$API_CAP" > "$_out"
  elif command -v wget >/dev/null 2>&1; then
    wget -q -T "$CMD_TIMEOUT" -O - "$_url" 2>>"$ERRORS" | head -c "$API_CAP" > "$_out"
  fi
  if [ -s "$_out" ]; then
    sanitize_file "$_out"
    manifest_add "$_rel" "api" "$3" "$_title"
  else
    rm -f "$_out"
  fi
}

# --- environment detection ------------------------------------------------------
NETDATA_PID=$(pidof netdata 2>/dev/null | awk '{print $1}')
[ -z "${NETDATA_PID:-}" ] && NETDATA_PID=$(ps -eo pid=,comm= 2>/dev/null | awk '$2=="netdata"{print $1; exit}')

# path candidates per install type: FHS packages, static (/opt/netdata),
# FreeBSD ports (/usr/local + /var/db), Homebrew (incl. Apple Silicon prefix)
CONFDIR=""
for d in /etc/netdata /opt/netdata/etc/netdata /usr/local/etc/netdata /opt/homebrew/etc/netdata; do
  [ -d "$d" ] && { CONFDIR="$d"; break; }
done
LOGDIR=""
for d in /var/log/netdata /opt/netdata/var/log/netdata /usr/local/var/log/netdata /opt/homebrew/var/log/netdata; do
  [ -d "$d" ] && { LOGDIR="$d"; break; }
done
LIBDIR=""
for d in /var/lib/netdata /opt/netdata/var/lib/netdata /var/db/netdata /usr/local/var/lib/netdata /opt/homebrew/var/lib/netdata; do
  [ -d "$d" ] && { LIBDIR="$d"; break; }
done
CACHEDIR=""
for d in /var/cache/netdata /opt/netdata/var/cache/netdata /var/db/netdata/cache /usr/local/var/cache/netdata /opt/homebrew/var/cache/netdata; do
  [ -d "$d" ] && { CACHEDIR="$d"; break; }
done

NETDATA_BIN=$(command -v netdata 2>/dev/null)
[ -z "${NETDATA_BIN:-}" ] && [ -n "${NETDATA_PID:-}" ] && [ -r "/proc/$NETDATA_PID/exe" ] && \
  NETDATA_BIN=$(readlink -f "/proc/$NETDATA_PID/exe" 2>/dev/null)
[ -z "${NETDATA_BIN:-}" ] && [ -x /opt/netdata/usr/sbin/netdata ] && NETDATA_BIN=/opt/netdata/usr/sbin/netdata

if command -v curl >/dev/null 2>&1 && curl -sf --max-time 3 "http://127.0.0.1:${NDPORT}/api/v1/info" >/dev/null 2>&1; then
  api_ok=1
elif command -v wget >/dev/null 2>&1 && wget -q -T 3 -O /dev/null "http://127.0.0.1:${NDPORT}/api/v1/info" 2>/dev/null; then
  api_ok=1
fi

IS_CONTAINER=0
[ -f /.dockerenv ] && IS_CONTAINER=1
grep -qE '(docker|containerd|kubepods|lxc)' /proc/1/cgroup 2>/dev/null && IS_CONTAINER=1

info "netdata-sos $VERSION"
info "agent pid: ${NETDATA_PID:-not running} | api: $([ $api_ok = 1 ] && echo up || echo unreachable) | config: ${CONFDIR:-not found} | container: $IS_CONTAINER"

# =============================================================================
# 01-system
# =============================================================================
info "collecting: system"
collect_cmd 01-system/uname.txt            "Kernel and architecture" uname -a
collect_file 01-system/os-release.txt      "OS distribution" /etc/os-release
collect_cmd 01-system/uptime-load.txt      "Uptime and load" uptime
collect_cmd 01-system/cpu-count.txt        "CPU count" sh -c 'nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || getconf _NPROCESSORS_ONLN 2>/dev/null'
collect_cmd 01-system/memory.txt           "Memory overview" sh -c '
  if command -v free >/dev/null; then free -m;
  elif [ -r /proc/meminfo ]; then head -6 /proc/meminfo;
  else sysctl -n hw.memsize hw.physmem 2>/dev/null; command -v vm_stat >/dev/null && vm_stat; fi 2>/dev/null; true'
collect_cmd 01-system/disk-usage.txt       "Filesystem usage" df -h
collect_cmd 01-system/virtualization.txt   "Virtualization/container detection" sh -c 'command -v systemd-detect-virt >/dev/null && systemd-detect-virt || echo "systemd-detect-virt not available"'
[ -d /sys/fs/cgroup ] && collect_cmd 01-system/cgroups.txt "cgroup version" stat -fc %T /sys/fs/cgroup
collect_cmd 01-system/clock-timesync.txt   "Clock and time sync (drift breaks streaming/cloud)" sh -c 'date -u; command -v timedatectl >/dev/null && timedatectl status || true'
collect_cmd 01-system/mountinfo.txt        "Mount table (namespace visibility issues)" sh -c 'cat /proc/self/mountinfo 2>/dev/null || mount'
collect_cmd 01-system/selinux-apparmor.txt "MAC status" sh -c '
  o="";
  command -v getenforce >/dev/null && o="selinux: $(getenforce 2>/dev/null)";
  [ -d /sys/kernel/security/apparmor ] && o="$o apparmor: present";
  [ -n "$o" ] && echo "$o" || echo "(no SELinux/AppArmor detected)"'
collect_cmd 01-system/kernel-messages.txt  "Kernel messages: OOM/segfault/netdata (evidence of kills and crashes)" \
  sh -c 'out=""; command -v journalctl >/dev/null && out=$(journalctl -k --no-pager --since "-'"$SINCE_HOURS"' hours" 2>/dev/null | grep -iE "oom|out of memory|segfault|netdata" | tail -300);
  [ -z "$out" ] && out=$(dmesg 2>/dev/null | grep -iE "oom|out of memory|segfault|netdata" | tail -300);
  if [ -n "$out" ]; then printf "%s\n" "$out"; else echo "(no matching kernel messages, or kernel log not readable in this environment)"; fi; true'

# =============================================================================
# 02-install
# =============================================================================
info "collecting: install"
for envf in "$CONFDIR/.environment" /etc/netdata/.environment /opt/netdata/etc/netdata/.environment; do
  [ -f "$envf" ] && { collect_file 02-install/environment-file.txt "Install-time environment (method, flags, channel; contains no secrets)" "$envf"; break; }
done
for itf in "$CONFDIR/.install-type" /etc/netdata/.install-type /opt/netdata/etc/netdata/.install-type; do
  [ -f "$itf" ] && { collect_file 02-install/install-type.file.txt "Install type marker (kickstart-build|kickstart-static|oci|custom|binpkg-*)" "$itf"; break; }
done
collect_cmd 02-install/package-info.txt "Netdata packages installed (name/version/status)" sh -c '
  found=0;
  if command -v dpkg-query >/dev/null; then
    out=$(dpkg-query -W -f "\${Package} \${Version} [\${Status}]\n" "*netdata*" 2>/dev/null);
    printf "%s\n" "$out";
    printf "%s" "$out" | grep -q "install ok installed" && found=1;
  fi;
  if command -v rpm >/dev/null; then
    out=$(rpm -qa "*netdata*" 2>/dev/null); [ -n "$out" ] && { printf "%s\n" "$out"; found=1; };
  fi;
  if command -v apk >/dev/null; then
    out=$(apk list --installed 2>/dev/null | grep -i netdata); [ -n "$out" ] && { printf "%s\n" "$out"; found=1; };
  fi;
  [ "$found" = "0" ] && echo "(no netdata OS package installed via dpkg/rpm/apk - normal for docker, static and from-source installs; a \"not-installed\" stub above just means another package references the name. See install-type.txt for how this agent was installed.)";
  true'
collect_cmd 02-install/install-type.txt "Install type inference" sh -c '
  o=0;
  [ -d /opt/netdata/etc/netdata ] && { echo "static build (/opt/netdata)"; o=1; };
  [ -f /.dockerenv ] && { echo "docker container (/.dockerenv present)"; o=1; };
  [ -f /etc/netdata/.environment ] && { echo "kickstart-managed (/etc/netdata/.environment present)"; o=1; };
  command -v netdata >/dev/null && { echo "netdata binary: $(command -v netdata)"; o=1; };
  [ "$o" = "0" ] && echo "(no netdata installation detected on this system)";
  true'
if [ "$IS_CONTAINER" = "1" ]; then
  collect_cmd 02-install/container-context.txt "Container context (pid1, env, cgroup)" sh -c '
    echo "== /proc/1/comm =="; cat /proc/1/comm 2>/dev/null;
    echo "== /proc/1/cgroup =="; cat /proc/1/cgroup 2>/dev/null;
    echo "== container env (NETDATA_*/DOCKER_*) ==";
    tr "\0" "\n" < /proc/1/environ 2>/dev/null | grep -E "^(NETDATA_|DOCKER_|DO_NOT)" \
      || env | grep -E "^(NETDATA_|DOCKER_|DO_NOT)" \
      || echo "(no NETDATA_*/DOCKER_* env vars visible)";
    true'
fi

# =============================================================================
# 03-process
# =============================================================================
info "collecting: process"
collect_cmd 03-process/ps-netdata.txt "Netdata process tree with CPU/memory" sh -c 'ps aux 2>/dev/null | head -1; ps aux 2>/dev/null | grep -E "[n]etdata|[g]o.d|[e]bpf|[a]pps.plugin|[c]harts.d|[p]ython.d" | grep -v "netdata-sos" | head -50'
if [ -n "${NETDATA_PID:-}" ]; then
  collect_cmd 03-process/threads-cpu.txt "Per-thread CPU of netdata (which thread is hot)" sh -c "
    if ps -L -o pid,tid,pcpu,pmem,comm -p $NETDATA_PID >/dev/null 2>&1; then
      ps -L -o pid,tid,pcpu,pmem,comm -p $NETDATA_PID | head -1;
      ps -L -o pid,tid,pcpu,pmem,comm -p $NETDATA_PID | tail -n +2 | sort -k3 -rn | head -40;
    else ps -M -p $NETDATA_PID 2>/dev/null | head -40 || ps -H -p $NETDATA_PID 2>/dev/null | head -40; fi; true"
  if [ -d "/proc/$NETDATA_PID" ]; then
    collect_cmd 03-process/proc-status.txt "Process status (RSS, threads, ctx switches)" cat "/proc/$NETDATA_PID/status"
    collect_cmd 03-process/proc-limits.txt "Process limits" cat "/proc/$NETDATA_PID/limits"
    collect_cmd 03-process/fd-count.txt "Open file descriptors" sh -c "ls /proc/$NETDATA_PID/fd 2>/dev/null | wc -l"
    collect_cmd 03-process/process-environ.txt "Netdata process environment (proxy/claim vars; values sanitized)" \
      sh -c "if tr '\0' '\n' < /proc/$NETDATA_PID/environ 2>/dev/null; then :; else
        echo '(/proc/$NETDATA_PID/environ not readable - containers need CAP_SYS_PTRACE for this)';
        echo '-- fallback: NETDATA_*/proxy vars visible to this shell (docker exec inherits container env) --';
        env | grep -iE '^(NETDATA_|https?_proxy|no_proxy|all_proxy)' || echo '(none)';
        echo '-- on the docker HOST you can also run: docker inspect -f \"{{.Config.Env}}\" <container> --';
      fi"
  fi
fi
collect_cmd 03-process/zombies.txt "Zombie processes (plugin reaping issues in containers)" \
  sh -c 'z=$(ps -eo pid=,ppid=,stat=,comm= 2>/dev/null | awk "\$3 ~ /Z/" | head -30); [ -n "$z" ] && printf "%s\n" "$z" || echo "(no zombie processes)"'

# =============================================================================
# 04-config
# =============================================================================
info "collecting: config"
if [ "$api_ok" = "1" ]; then
  collect_api 04-config/effective-netdata.conf "EFFECTIVE running config (merged, annotated) - authoritative over on-disk file" /netdata.conf
fi
if [ -n "$CONFDIR" ]; then
  collect_cmd 04-config/config-tree.txt "User config dir tree (files here = user-customized)" ls -laR "$CONFDIR"
  collect_file 04-config/netdata.conf "On-disk main config" "$CONFDIR/netdata.conf"
  collect_file 04-config/stream.conf "Streaming config (parent/child; api key redacted)" "$CONFDIR/stream.conf"
  collect_file 04-config/exporting.conf "Exporting engine config (credentials redacted)" "$CONFDIR/exporting.conf"
  collect_file 04-config/go.d.conf "go.d orchestrator config (module enable/disable)" "$CONFDIR/go.d.conf"
  # every user-customized collector/health config (these are the copies users made with edit-config)
  for sub in go.d health.d python.d charts.d statsd.d; do
    if [ -d "$CONFDIR/$sub" ]; then
      for f in "$CONFDIR/$sub"/*.conf "$CONFDIR/$sub"/*.yml "$CONFDIR/$sub"/*.yaml; do
        [ -f "$f" ] || continue
        collect_file "04-config/$sub/$(basename "$f")" "User-customized $sub config (secrets redacted)" "$f" 262144
      done
    fi
  done
  for f in "$CONFDIR"/*.conf; do
    [ -f "$f" ] || continue
    case "$(basename "$f")" in
      netdata.conf|stream.conf|exporting.conf|go.d.conf) ;;
      *) collect_file "04-config/$(basename "$f")" "Additional user config" "$f" 262144 ;;
    esac
  done
fi
if [ -n "$LIBDIR" ] && [ -f "$LIBDIR/cloud.d/cloud.conf" ]; then
  collect_file 04-config/cloud.conf "Cloud connection config (token redacted)" "$LIBDIR/cloud.d/cloud.conf"
fi

# =============================================================================
# 05-logs
# =============================================================================
info "collecting: logs (last ${SINCE_HOURS}h, capped)"
if command -v journalctl >/dev/null 2>&1; then
  collect_cmd 05-logs/journal-netdata.txt "systemd journal for netdata unit" \
    sh -c "journalctl -u netdata --no-pager -o short-iso --since '-${SINCE_HOURS} hours' 2>/dev/null | tail -c $LOG_CAP; true"
  collect_cmd 05-logs/journal-namespace-netdata.txt "netdata journal namespace (some installs log here)" \
    sh -c "journalctl --namespace=netdata --no-pager -o short-iso --since '-${SINCE_HOURS} hours' 2>/dev/null | tail -c $LOG_CAP; true"
fi
if [ -n "$LOGDIR" ]; then
  for lf in error.log daemon.log collector.log health.log aclk.log debug.log; do
    collect_file "05-logs/$lf" "Agent log file: $lf" "$LOGDIR/$lf" "$LOG_CAP"
  done
  collect_file 05-logs/access.log "API access log (clients pseudonymized)" "$LOGDIR/access.log" 1048576
  # docker images symlink logs to /dev/stdout|stderr - history only exists in `docker logs`
  DOCKER_LOGS_NEEDED=0
  if [ -L "$LOGDIR/daemon.log" ] || [ -L "$LOGDIR/error.log" ]; then
    _lt=$(readlink "$LOGDIR/daemon.log" 2>/dev/null || readlink "$LOGDIR/error.log" 2>/dev/null)
    case "$_lt" in /dev/std*)
      DOCKER_LOGS_NEEDED=1
      mkdir -p "$WORK/05-logs"
      cat > "$WORK/05-logs/LOGS-ARE-IN-DOCKER.txt" <<'EOF'
This agent logs to the container's stdout/stderr. Its log history is NOT
available from inside the container. To complete this bundle, ALSO run on
the docker host and attach the output:

    docker logs --since 24h <netdata-container> > netdata-docker.log 2>&1
EOF
      manifest_add 05-logs/LOGS-ARE-IN-DOCKER.txt file generated "Instruction: agent logs live in 'docker logs' on the host"
      ;;
    esac
  fi
fi
if command -v journalctl >/dev/null 2>&1; then
  collect_cmd 05-logs/journal-updater.txt "Auto-updater service journal (updater keeps no persistent log file)" \
    sh -c "journalctl -u netdata-updater.service --no-pager -o short-iso 2>/dev/null | tail -200; true"
fi
collect_cmd 05-logs/coredumps.txt "Recent coredump METADATA for netdata (not the dumps)" \
  sh -c 'if command -v coredumpctl >/dev/null; then coredumpctl list --no-pager 2>/dev/null | awk "NR==1 || tolower(\$0) ~ /netdata/" | tail -21; else echo "coredumpctl not available"; fi; true'

# =============================================================================
# 06-state
# =============================================================================
info "collecting: state"
# status file: agent writes to first writable of these; newest mtime wins (status-file-io.c)
NEWEST_STATUS=""
for sf in "$LIBDIR/status-netdata.json" "$CACHEDIR/status-netdata.json" /tmp/status-netdata.json /run/status-netdata.json /var/run/status-netdata.json; do
  [ -f "$sf" ] || continue
  if [ -z "$NEWEST_STATUS" ] || [ "$sf" -nt "$NEWEST_STATUS" ]; then NEWEST_STATUS="$sf"; fi
done
[ -n "$NEWEST_STATUS" ] && collect_file 06-state/status-file.json "Daemon status file: LAST EXIT/CRASH RECORD incl. fatal stack trace (read this first for crashes)" "$NEWEST_STATUS"
if [ -n "$LIBDIR" ]; then
  # bearer_tokens/ FILENAMES are live API tokens - withhold them from the listing
  collect_cmd 06-state/state-tree.txt "State dir listing (bearer token filenames withheld - they are live tokens)" \
    sh -c 'ls -laR '"$LIBDIR"' 2>/dev/null | awk "
      /\/bearer_tokens:\$/ { print; skip=1; n=0; next }
      skip && /^\$/         { print \"  [\" n \" token file(s) - names withheld, they ARE the tokens]\"; print; skip=0; next }
      skip && /^total/      { next }
      skip                  { if (\$0 !~ / \.\.?\$/) n++; next }
                            { print }"'
  collect_cmd 06-state/cloud-state.txt "Cloud claim state (claimed_id is safe; token/private.pem are never collected)" sh -c '
    echo "== cloud.d listing =="; ls -la '"$LIBDIR"'/cloud.d/ 2>/dev/null;
    echo "== claimed_id ==";
    cat '"$LIBDIR"'/cloud.d/claimed_id 2>/dev/null || echo "(no claimed_id file - agent not claimed)"; echo;
    echo "(token and private.pem intentionally NOT collected)"; true'
  collect_file 06-state/health-silencers.json "Persisted alert silencers" "$LIBDIR/health.silencers.json"
  for gjs in "$LIBDIR"/god-jobs-statuses.json "$LIBDIR"/*jobs-statuses*.json; do
    [ -f "$gjs" ] && { collect_file 06-state/go.d-job-statuses.json "go.d collector job states (which jobs run/fail)" "$gjs"; break; }
  done
  if [ -d "$LIBDIR/config" ]; then
    for dc in "$LIBDIR/config"/*.dyncfg; do
      [ -f "$dc" ] || continue
      collect_file "06-state/dyncfg/$(basename "$dc")" "Dynamic config created via UI/API (secrets redacted)" "$dc" 262144
    done
  fi
fi
if [ -n "$CACHEDIR$LIBDIR" ]; then
  collect_cmd 06-state/db-disk-usage.txt "Database disk usage per tier + sqlite sizes + corruption sentinels" sh -c '
    [ -n "'"$CACHEDIR"'" ] && du -sh '"$CACHEDIR"'/* 2>/dev/null | sort -rh | head -30;
    [ -n "'"$LIBDIR"'" ] && ls -la '"$LIBDIR"'/*.db* 2>/dev/null;
    echo "== sqlite corruption/recovery sentinels (presence = past corruption) ==";
    ls -la '"$CACHEDIR"'/*.bad* '"$CACHEDIR"'/.*.recover '"$CACHEDIR"'/*.recover 2>/dev/null || echo "(none found)";
    true'
fi

# =============================================================================
# 07-runtime (only when the agent responds)
# =============================================================================
if [ "$api_ok" = "1" ]; then
  info "collecting: runtime (agent is up)"
  collect_api 07-runtime/info-v3.json "BEST SINGLE CALL: buildinfo, features, cloud status, per-tier retention (works even under bearer protection)" /api/v3/info
  collect_api 07-runtime/info-v1.json "Agent info v1: version, cloud/stream booleans, mirrored hosts" /api/v1/info
  collect_api 07-runtime/node-instances.json "Node instances: children, streaming state, db_size per tier, metric counts" /api/v2/node_instances
  collect_api 07-runtime/stream-info.json "Streaming diagnostics" /api/v3/stream_info
  collect_api 07-runtime/aclk.json "Cloud/ACLK connection state" /api/v1/aclk
  collect_api 07-runtime/alerts-active.json "Currently raised alerts" "/api/v3/alerts?options=active"
  collect_api 07-runtime/alerts-all.json "All alert instances (summary)" "/api/v1/alarms?all"
  collect_api 07-runtime/functions.json "Registered functions (which plugins expose what)" /api/v1/functions
  collect_api 07-runtime/ml-info.json "Machine learning status" /api/v1/ml_info
  # netdata's own resource usage, bounded windows (perf triage without screenshots)
  collect_api 07-runtime/self-cpu.csv "Netdata CPU last 10min (csv)" "/api/v1/data?chart=netdata.server_cpu&after=-600&points=60&format=csv"
  collect_api 07-runtime/self-memory.csv "Netdata memory last 10min (csv)" "/api/v1/data?chart=netdata.memory&after=-600&points=60&format=csv"
  collect_api 07-runtime/self-api-clients.csv "Netdata API clients last 10min (csv)" "/api/v1/data?chart=netdata.clients&after=-600&points=60&format=csv"
else
  info "agent API unreachable - skipping runtime section"
  mkdir -p "$WORK/07-runtime"
  echo "Agent API at 127.0.0.1:$NDPORT was unreachable when this bundle was created. See 05-logs and 06-state/status-file.json for why." > "$WORK/07-runtime/AGENT-WAS-DOWN.txt"
  manifest_add 07-runtime/AGENT-WAS-DOWN.txt "file" "generated" "Marker: agent API unreachable at collection time"
fi
if [ -n "${NETDATA_BIN:-}" ]; then
  collect_cmd 07-runtime/buildinfo.txt "netdata -W buildinfo (verbatim - paths section matters; works with daemon down)" "$NETDATA_BIN" -W buildinfo
  collect_cmd_raw 07-runtime/buildinfo.json "netdata -W buildinfojson (machine-readable; no header so it parses as JSON)" "$NETDATA_BIN" -W buildinfojson
fi
if command -v netdatacli >/dev/null 2>&1 && [ -n "${NETDATA_PID:-}" ]; then
  collect_cmd_raw 07-runtime/aclk-state.json "Cloud connectivity state (netdatacli aclk-state json; no header so it parses as JSON)" netdatacli aclk-state json
fi

# =============================================================================
# 08-network
# =============================================================================
info "collecting: network"
collect_cmd 08-network/listening-sockets.txt "Listening sockets (netdata-related)" \
  sh -c 'if command -v ss >/dev/null; then ss -tlnp 2>/dev/null | awk "NR==1 || /19999|netdata/";
  elif command -v sockstat >/dev/null; then sockstat -l 2>/dev/null | awk "NR==1 || /19999|netdata/";
  else netstat -an 2>/dev/null | grep -i listen | grep 19999; fi; true'
if [ "$OBFUSCATE" = "1" ]; then
  # search/domain values are often corporate-internal names outside private TLDs
  collect_cmd 08-network/resolv-conf.txt "DNS resolver config (search domains withheld)" \
    sh -c 'sed -E "s/^((search|domain)[ \t]).*/\1[SEARCH-DOMAINS-WITHHELD]/" /etc/resolv.conf 2>/dev/null; true'
else
  collect_file 08-network/resolv-conf.txt "DNS resolver config" /etc/resolv.conf
fi
collect_cmd 08-network/proxy-env.txt "Proxy environment (this shell; see 03-process/process-environ.txt for the agent view)" \
  sh -c 'env | grep -iE "^(https?_proxy|no_proxy|all_proxy)=" || echo "(no proxy variables set)"'
collect_cmd 08-network/cloud-connectivity.txt "Reachability of Netdata Cloud (DNS + TLS + response code, no data sent)" sh -c '
  if command -v curl >/dev/null; then
    curl -sv --max-time 8 -o /dev/null https://app.netdata.cloud/ 2>&1 \
      | grep -E "^\*|^< HTTP|^> " \
      | grep -viE "TLS handshake|change.?cipher|certificate|subject:|issuer:|ALPN|offering|CAfile|user-agent|accept:" \
      | head -40;
  elif command -v wget >/dev/null; then
    if wget -q -T 8 -O /dev/null https://app.netdata.cloud/ 2>/dev/null;
    then echo "wget https://app.netdata.cloud/ : SUCCESS";
    else echo "wget https://app.netdata.cloud/ : FAILED (exit $?)"; fi;
  else echo "neither curl nor wget available"; fi; true'

# =============================================================================
# summary + manifest + README
# =============================================================================
info "writing summary and manifest"
AGENT_VERSION=$(awk -F'"' '/"version"/{print $4; exit}' "$WORK/07-runtime/info-v1.json" 2>/dev/null)
[ -z "$AGENT_VERSION" ] && [ -n "${NETDATA_BIN:-}" ] && AGENT_VERSION=$("$NETDATA_BIN" -v 2>/dev/null | head -1)
CLAIMED="unknown"
for _aclkf in "$WORK/07-runtime/aclk-state.json" "$WORK/07-runtime/aclk.json"; do
  [ -f "$_aclkf" ] || continue
  if grep -q '"agent-claimed":true' "$_aclkf" 2>/dev/null; then CLAIMED="yes"; break; fi
  if grep -q '"agent-claimed":false' "$_aclkf" 2>/dev/null; then CLAIMED="no"; break; fi
done
[ "$CLAIMED" = "unknown" ] && [ -f "$WORK/06-state/cloud-state.txt" ] && \
  grep -qE '^[0-9a-f-]{36}' "$WORK/06-state/cloud-state.txt" && CLAIMED="yes"
ERRCOUNT=""
[ -f "$WORK/05-logs/error.log" ] && ERRCOUNT=$(grep -ci error "$WORK/05-logs/error.log" 2>/dev/null)
CRASH_HINT=""
[ -f "$WORK/06-state/status-file.json" ] && CRASH_HINT=$(awk -F'"' '/"exit_reason"|"cause"/{print $4}' "$WORK/06-state/status-file.json" 2>/dev/null | head -1)

{
  echo "NETDATA SUPPORT BUNDLE SUMMARY"
  echo "generated:        $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "tool version:     $VERSION"
  echo "runtime seconds:  $(( $(now_s) - START_TS ))"
  echo "ran as root:      $([ "$(id -u)" = "0" ] && echo yes || echo no)"
  echo "pii obfuscation:  $([ "$OBFUSCATE" = "1" ] && echo on || echo OFF)"
  echo
  echo "agent version:    ${AGENT_VERSION:-unknown}"
  _agent_note=""
  if [ -n "${NETDATA_PID:-}" ] && [ "$IS_CONTAINER" = "0" ] && [ -z "$CONFDIR" ] && \
     grep -qE 'docker|containerd|kubepods|lxc' "/proc/$NETDATA_PID/cgroup" 2>/dev/null; then
    _agent_note=" (process appears to run INSIDE a container; no local install found on this host)"
  fi
  echo "agent running:    $([ -n "${NETDATA_PID:-}" ] && echo "yes (pid $NETDATA_PID)$_agent_note" || echo NO)"
  if [ -z "${NETDATA_PID:-}" ] && grep -q '"status":"running"' "$WORK/06-state/status-file.json" 2>/dev/null; then
    echo "WARNING: status file still says 'running' but no netdata process exists -"
    echo "         unclean termination (SIGKILL / OOM kill / power loss); the agent"
    echo "         could not update the file at death. Check 01-system/kernel-messages.txt."
  fi
  echo "agent api:        $([ "$api_ok" = "1" ] && echo reachable || echo UNREACHABLE)"
  echo "container:        $([ "$IS_CONTAINER" = "1" ] && echo yes || echo no)"
  echo "config dir:       ${CONFDIR:-NOT FOUND}"
  echo "claimed to cloud: $CLAIMED"
  [ -n "$CRASH_HINT" ] && echo "last exit reason: $CRASH_HINT   <-- check 06-state/status-file.json"
  [ -n "$ERRCOUNT" ] && echo "error.log 'error' lines: $ERRCOUNT"
  [ "${DOCKER_LOGS_NEEDED:-0}" = "1" ] && echo "NOTE: agent log HISTORY is not in this bundle - it lives in 'docker logs' on the host (see 05-logs/LOGS-ARE-IN-DOCKER.txt)"
  echo
  echo "READ ORDER FOR TRIAGE:"
  echo "  crashes/won't start -> 06-state/status-file.json, 05-logs/, 01-system/kernel-messages.txt"
  echo "  collector issues    -> 04-config/go.d*, 05-logs/collector.log"
  echo "  streaming issues    -> 04-config/stream.conf, 07-runtime/node-instances.json, 01-system/clock-timesync.txt"
  echo "  cloud/claiming      -> 06-state/cloud-state.txt, 07-runtime/aclk-state.json, 08-network/"
  echo "  performance         -> 03-process/threads-cpu.txt, 06-state/db-disk-usage.txt, 07-runtime/node-instances.json"
} > "$WORK/summary.txt"
manifest_add summary.txt file generated "Human summary"

cat > "$WORK/README.md" <<'EOF'
# Netdata Support Bundle

Generated by `netdata-sos`. Contents are SANITIZED:
secrets (tokens, api keys, passwords) are always redacted; by default IPs,
MACs, emails and hostnames are replaced with stable pseudonyms (`ip-1`,
`redacted-host`, `[EMAIL]`, `[MAC]`) - consistent across all files, so
cross-referencing still works. The pseudonym map stays on the user's machine,
next to the tarball - it is NOT in this bundle.

## Layout (triage order)

| dir | contents |
|---|---|
| `summary.txt` | one-page overview - start here |
| `MANIFEST.json` | machine-readable index of every file (origin, size, sanitization) |
| `01-system/` | OS, kernel, memory, disks, virtualization, clock sync, OOM/segfault evidence |
| `02-install/` | install method, packages, .environment, container context |
| `03-process/` | netdata process tree, per-thread CPU, limits, fds, environment |
| `04-config/` | effective (running) config + every user-customized config file |
| `05-logs/` | journal + agent log files (window-capped), updater log, coredump metadata |
| `06-state/` | daemon status file (crash record), state/db disk usage, claim state |
| `07-runtime/` | live API captures: info, node instances, alerts, aclk state, buildinfo |
| `08-network/` | listening sockets, DNS, proxy, Netdata Cloud reachability |

## Conventions

- Command captures (`*.txt`) begin with a `# netdata-sos | command: ...`
  provenance header and end with `# exit: N`.
- Copied files (configs, logs, json) are pristine (no injected headers);
  their origin is recorded in `MANIFEST.json`.
- `07-runtime/AGENT-WAS-DOWN.txt` exists only when the agent was not running.
EOF
manifest_add README.md file generated "Bundle documentation"

# emit MANIFEST.json LAST so every file (incl. summary.txt and README.md) is indexed
{
  echo '{'
  echo '  "schema": "netdata-sos-bundle/v1",'
  echo "  \"tool_version\": \"$VERSION\","
  echo "  \"generated_utc\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\","
  echo "  \"runtime_seconds\": $(( $(now_s) - START_TS )),"
  echo "  \"pii_obfuscated\": $([ "$OBFUSCATE" = "1" ] && echo true || echo false),"
  echo "  \"secrets_redacted\": true,"
  echo "  \"agent_running\": $([ -n "${NETDATA_PID:-}" ] && echo true || echo false),"
  echo "  \"agent_api_reachable\": $([ "$api_ok" = "1" ] && echo true || echo false),"
  echo "  \"is_container\": $([ "$IS_CONTAINER" = "1" ] && echo true || echo false),"
  echo '  "files": ['
  sed '$!s/$/,/' "$MANIFEST_ROWS" | sed 's/^/    /'
  echo '  ]'
  echo '}'
} > "$WORK/MANIFEST.json"

# =============================================================================
# tarball
# =============================================================================
mkdir -p "$OUTDIR"
TARBALL="$OUTDIR/$BUNDLE.tar.gz"
# never write through a pre-existing path (symlink attack on shared tmp dirs)
if [ -e "$TARBALL" ] || [ -L "$TARBALL" ]; then
  echo "refusing to overwrite existing $TARBALL" >&2
  exit 1
fi
( cd "$STAGING" && tar czf "$TARBALL" "$BUNDLE" ) || { echo "failed to create tarball" >&2; exit 1; }
chmod 600 "$TARBALL" 2>/dev/null

if [ "$OBFUSCATE" = "1" ] && [ -s "$MAP_FILE" ]; then
  cp "$MAP_FILE" "$OUTDIR/$BUNDLE.pseudonym-map.tsv" 2>/dev/null && chmod 600 "$OUTDIR/$BUNDLE.pseudonym-map.tsv"
fi

TOTAL_S=$(( $(now_s) - START_TS ))
SIZE=$(du -h "$TARBALL" 2>/dev/null | cut -f1)
echo >&2
info "done in ${TOTAL_S}s"
info "bundle:  $TARBALL ($SIZE)"
[ "$OBFUSCATE" = "1" ] && [ -s "$MAP_FILE" ] && info "pseudonym map (KEEP PRIVATE, do not send): $OUTDIR/$BUNDLE.pseudonym-map.tsv"
info "review it:  tar -tzf $TARBALL"
if [ "${DOCKER_LOGS_NEEDED:-0}" = "1" ]; then
  info "IMPORTANT: this agent logs to the container's stdout - its log history is NOT in this bundle."
  info "on the docker HOST also run:  docker logs --since 24h <netdata-container> > netdata-docker.log 2>&1"
  info "and attach netdata-docker.log to the ticket as well."
fi
info "attach the bundle to your support ticket."
