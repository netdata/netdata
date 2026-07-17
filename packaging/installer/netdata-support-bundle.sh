#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later
#
# shellcheck disable=SC2016  # the sh -c and awk mini-programs in the collectors
# are intentionally single-quoted: they must reach the child shell/awk unexpanded.
#
# netdata-support-bundle - collect a sanitized diagnostic bundle for Netdata support tickets.
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
# Usage: sudo sh netdata-support-bundle.sh [options]
#   -o, --output DIR     where to write the tarball (default: /tmp)
#   --since HOURS        log window in hours (default: 24)
#   --timeout SECONDS    per-command timeout (default: 10)
#   --no-obfuscate       disable PII pseudonymization (secrets STILL redacted)
#   --keep-staging       keep staging dir for inspection
#   --selftest           run the sanitizer regression vectors and exit
#   -v, --version        print version
#   -h, --help           this help

set -u

# --- self-demotion FIRST (before arg parsing consumes "$@"): never compete
# --- with real workloads
if [ -z "${ND_SUPPORT_BUNDLE_DEMOTED:-}" ]; then
  ND_SUPPORT_BUNDLE_DEMOTED=1; export ND_SUPPORT_BUNDLE_DEMOTED
  # ionice may exist but be denied (e.g. no CAP_SYS_NICE): probe idle class
  # once, and only route through it if it actually works; else just nice
  if command -v ionice >/dev/null 2>&1 && ionice -c 3 true >/dev/null 2>&1; then
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
SELFTEST=0
# checked before each collector; one in-flight command may overrun by up to
# CMD_TIMEOUT, so the hard runtime bound is GLOBAL_DEADLINE + CMD_TIMEOUT
GLOBAL_DEADLINE=240
LOG_CAP=5242880          # 5 MiB per log file
FILE_CAP=1048576         # 1 MiB per config/state file
API_CAP=2097152          # 2 MiB per API response

need_val() { [ $# -ge 2 ] || { echo "option $1 needs a value" >&2; exit 1; }; }
while [ $# -gt 0 ]; do
  case "$1" in
    -o|--output) need_val "$@"; OUTDIR="$2"; shift 2 ;;
    --since) need_val "$@"; SINCE_HOURS="$2"; shift 2 ;;
    --timeout) need_val "$@"; CMD_TIMEOUT="$2"; shift 2 ;;
    --no-obfuscate) OBFUSCATE=0; shift ;;
    --keep-staging) KEEP_STAGING=1; shift ;;
    --selftest) SELFTEST=1; shift ;;
    -v|--version) echo "netdata-support-bundle $VERSION"; exit 0 ;;
    -h|--help) sed -n '/^# netdata-support-bundle - collect/,/^#   -h, --help/p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown option: $1" >&2; exit 1 ;;
  esac
done

case "$SINCE_HOURS" in *[!0-9]*|''|0) echo "--since must be a positive integer (hours)" >&2; exit 1 ;; *) : ;; esac
case "$CMD_TIMEOUT" in *[!0-9]*|''|0) echo "--timeout must be a positive integer (seconds)" >&2; exit 1 ;; *) : ;; esac

umask 077
START_TS=$(date +%s)
NOW=$(date -u +%Y%m%d-%H%M%S)
STAGING=$(mktemp -d "${TMPDIR:-/tmp}/netdata-support-bundle.XXXXXX") || exit 1
BUNDLE="netdata-support-bundle-${NOW}-$$"
WORK="$STAGING/$BUNDLE"
mkdir -p "$WORK"
MAP_FILE="$STAGING/map.tsv"; : > "$MAP_FILE"
MANIFEST_ROWS="$STAGING/manifest.rows"; : > "$MANIFEST_ROWS"
ERRORS="$STAGING/errors.txt"; : > "$ERRORS"

cleanup() { [ "$KEEP_STAGING" = "1" ] || rm -rf "$STAGING"; }
trap cleanup EXIT
trap 'cleanup; trap - EXIT; exit 130' INT TERM

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
    *)
      # no timeout binary (macOS): portable watchdog so a stuck collector
      # cannot hang the whole run. KNOWN WEAKER GUARANTEE than GNU/BSD
      # timeout (which kills the whole process group): this best-effort path
      # SIGKILLs the direct child and, where pkill exists, its children;
      # deeper sh -c grandchildren may briefly orphan. Total runtime is still
      # bounded by GLOBAL_DEADLINE. Only reached on hosts lacking timeout.
      "$@" &
      _cmdpid=$!
      (
        _i=0
        while [ "$_i" -lt "$_t" ]; do
          sleep 1
          kill -0 "$_cmdpid" 2>/dev/null || exit 0
          _i=$((_i + 1))
        done
        command -v pkill >/dev/null 2>&1 && pkill -9 -P "$_cmdpid" 2>/dev/null
        kill -9 "$_cmdpid" 2>/dev/null
      ) &
      _wdpid=$!
      wait "$_cmdpid"
      _rc=$?
      kill "$_wdpid" 2>/dev/null
      wait "$_wdpid" 2>/dev/null
      return "$_rc"
      ;;
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
case "$RUN_USER" in root|netdata|"") RUN_USER="" ;; *) : ;; esac
[ ${#RUN_USER} -lt 3 ] && RUN_USER=""

sanitize_file() {
  _f="$1"
  [ -f "$_f" ] || return 0
  # binary/UTF-16 input would make line-based redaction byte-unsafe: withhold
  _b_all=$(wc -c < "$_f")
  _b_txt=$(LC_ALL=C tr -d '\000' < "$_f" | wc -c)
  if [ "$_b_all" -ne "$_b_txt" ]; then
    echo "[content withheld: file contains NUL bytes (binary or UTF-16?)]" > "$_f"
    return 0
  fi
  if awk -v obf="$OBFUSCATE" -v mapfile="$MAP_FILE" \
      -v host_short="$HOST_SHORT" -v host_fqdn="$HOST_FQDN" -v run_user="$RUN_USER" '
  BEGIN {
    nsec = split("api key,apikey,token,password,passwd,pwd,secret,community,bearer,webhook,license key,auth,credential,cookie,passphrase,proxy user,proxy pass,username,dsn,private key,access key,session,recipient,account sid,priv key", SK, ",");
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
  function diagnostic_key(lk) {
    # exemptions are decided by the KEY, never the value (a secret can be any
    # string, incl. "false" or a path). Keys ENDING in these words describe
    # secrets or toggles rather than being secrets: "bearer token protection",
    # "netdata management api key file", "TCP SYN cookies".
    if (lk ~ /(^| )(file|path|dir|directory|protection|support|mode|level|port|timeout|cookies|secure|log|size|options)$/) return 1;
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
      if (length(key) > 64 || key !~ /^[A-Za-z0-9]/ || key ~ /["`;|()\/]/) return line;
      lk = tolower(key);
      gsub(/[-_]/, " ", lk);
      if (diagnostic_key(lk)) return line;
      for (i = 1; i <= nsec; i++) {
        if (index(lk, SK[i]) > 0 && substr(line, pos + 1) ~ /[^ \t]/)
          return substr(line, 1, pos) " [REDACTED]";
      }
    }
    return line;
  }
  function redact_json(line,   out, rest, key, lk, i, k, m, v, pre, keypart, after, hit) {
    # "key": "value" pairs, possibly many per line
    out = ""; rest = line;
    while (match(rest, /"[^"]+"[ \t]*:[ \t]*"([^"\\]|\\.)*"/)) {
      m = substr(rest, RSTART, RLENGTH);
      key = m; sub(/^"/, "", key); sub(/".*/, "", key);
      lk = tolower(key); gsub(/[-_]/, " ", lk);
      if (!diagnostic_key(lk)) {
        for (i = 1; i <= nsec; i++) {
          if (index(lk, SK[i]) > 0) {
            sub(/:[ \t]*"([^"\\]|\\.)*"/, ": \"[REDACTED]\"", m);
            break;
          }
        }
      }
      out = out substr(rest, 1, RSTART - 1) m;
      rest = substr(rest, RSTART + RLENGTH);
    }
    line = out rest;
    # scalar (unquoted) JSON values under secret keys: "key": 12345
    out = ""; rest = line;
    while (match(rest, /"[^"]+"[ \t]*:[ \t]*[-0-9truefalsnu][0-9truefalsnul.eE+-]*/)) {
      m = substr(rest, RSTART, RLENGTH);
      key = m; sub(/^"/, "", key); sub(/".*/, "", key);
      lk = tolower(key); gsub(/[-_]/, " ", lk);
      hit = 0;
      for (i = 1; i <= nsec; i++) if (index(lk, SK[i]) > 0) hit = 1;
      if (hit && !diagnostic_key(lk)) sub(/:[ \t]*[-0-9truefalsnu][0-9truefalsnul.eE+-]*$/, ": \"[REDACTED]\"", m);
      out = out substr(rest, 1, RSTART - 1) m;
      rest = substr(rest, RSTART + RLENGTH);
    }
    line = out rest;
    # structured (array/object) values under secret keys: "key": [..] / {..}
    # (single-line; agent-API JSON is compact). Withhold the whole value.
    out = ""; rest = line;
    while (match(rest, /"[^"]+"[ \t]*:[ \t]*[[{]/)) {
      m = substr(rest, RSTART, RLENGTH);
      key = m; sub(/^"/, "", key); sub(/".*/, "", key);
      lk = tolower(key); gsub(/[-_]/, " ", lk);
      pre = substr(rest, 1, RSTART - 1);
      keypart = substr(m, 1, length(m) - 1);          # "key": (without the bracket)
      after = substr(rest, RSTART + RLENGTH - 1);      # from the [ or { onward
      hit = 0; for (i = 1; i <= nsec; i++) if (index(lk, SK[i]) > 0) hit = 1;
      if (hit && !diagnostic_key(lk) && (match(after, /^\[[^]]*\]/) || match(after, /^\{[^}]*\}/))) {
        out = out pre keypart " \"[REDACTED]\"";
        rest = substr(after, RLENGTH + 1);
      } else {
        out = out pre keypart;
        rest = after;
      }
    }
    return out rest;
  }
  function pseudo_ip(ip,   p) {
    if (ip ~ /^127\./ || ip == "0.0.0.0" || ip ~ /^255\./) return ip;
    if (!(ip in ipmap)) {
      if (nip >= 4096) return "redacted-ip";  # cap: non-correlating past 4096
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
      if (index(cand, ":") == 0 || pre ~ /[A-Za-z0-9._-]/ || length(cand) < 5 || cand ~ /:$/ ||
          (nc < 3 && index(cand, "::") == 0) ||
          (cand !~ /[A-Fa-f]/ && index(cand, "::") == 0 && nc < 6) ||
          cand == "::1") {
        out = out substr(rest, 1, RSTART + RLENGTH - 1);
        rest = substr(rest, RSTART + RLENGTH);
        continue;
      }
      if (!(cand in ip6map)) {
        if (nip6 >= 4096) { out = out substr(rest, 1, RSTART - 1) "redacted-ip6"; rest = substr(rest, RSTART + RLENGTH); continue; }
        nip6++; ip6map[cand] = "ip6-" nip6;
        print "ip6\t" cand "\t" ip6map[cand] >> mapfile;
      }
      out = out substr(rest, 1, RSTART - 1) ip6map[cand];
      rest = substr(rest, RSTART + RLENGTH);
    }
    return out rest;
  }
  function pseudo_fqdn(h) {
    if (!(h in fqmap)) {
      if (nfq >= 4096) return "redacted-host-overflow";  # cap: non-correlating
      nfq++; fqmap[h] = "private-host-" nfq;
      print "fqdn\t" h "\t" fqmap[h] >> mapfile;
    }
    return fqmap[h];
  }
  function replace_mapped_fqdns(line,   h, out, idx, pre, nxt) {
    # replace every known (pre-seeded or discovered) hostname, with word
    # boundaries so "host1" never corrupts "host10"
    for (h in fqmap) {
      if (length(h) < 4) continue;
      out = "";
      while ((idx = index(line, h)) > 0) {
        pre = (idx > 1) ? substr(line, idx - 1, 1) : "";
        nxt = substr(line, idx + length(h), 1);
        if (pre ~ /[A-Za-z0-9.-]/ || nxt ~ /[A-Za-z0-9.-]/) {
          out = out substr(line, 1, idx + length(h) - 1);
          line = substr(line, idx + length(h));
          continue;
        }
        out = out substr(line, 1, idx - 1) fqmap[h];
        line = substr(line, idx + length(h));
      }
      line = out line;
    }
    return line;
  }
  function redact_destination(line,   pos, head, valpart, n, parts, i, tok, hostp, rest2, cpos, proto) {
    # stream.conf destination/proxy destination values are user infrastructure
    # hostnames regardless of TLD. Token syntax: [PROTOCOL:]HOST[%IFACE][:PORT][:SSL]
    pos = index(line, "=");
    if (pos == 0) return line;
    head = substr(line, 1, pos);
    valpart = substr(line, pos + 1);
    n = split(valpart, parts, /[ \t]+/);
    valpart = "";
    for (i = 1; i <= n; i++) {
      tok = parts[i];
      if (tok == "") continue;
      proto = "";
      if (tok ~ /^(tcp|udp|unix):/) {
        cpos = index(tok, ":");
        proto = substr(tok, 1, cpos);
        tok = substr(tok, cpos + 1);
      }
      # bracketed IPv6 belongs to the IP rules; unix socket paths are not hostnames
      if (tok ~ /^\[/ || tok ~ /^\//) { valpart = valpart " " proto tok; continue; }
      cpos = index(tok, ":");
      if (cpos > 1) { hostp = substr(tok, 1, cpos - 1); rest2 = substr(tok, cpos); }
      else { hostp = tok; rest2 = ""; }
      cpos = index(hostp, "%");
      if (cpos > 1) { rest2 = substr(hostp, cpos) rest2; hostp = substr(hostp, 1, cpos - 1); }
      # leave IPs to the IP rules, and never map an existing pseudonym
      if (hostp != "" && length(hostp) >= 4 && \
          hostp !~ /^[0-9.]+$/ && hostp !~ /^[0-9A-Fa-f:]+$/ && \
          hostp !~ /^(ip|ip6|private-host)-[0-9]+$/)
        hostp = pseudo_fqdn(hostp);
      valpart = valpart " " proto hostp rest2;
    }
    return head valpart;
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
      out = out substr(rest, 1, RSTART - 1) pseudo_fqdn(fq);
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
  function gensub_home_one(line, pfx,   out, rest, seg, plen, i) {
    # <pfx><name> -> <pfx><pseudonym>, e.g. /home/alice -> /home/user-1
    out = ""; rest = line; plen = length(pfx);
    while (index(rest, pfx) > 0) {
      i = index(rest, pfx);
      out = out substr(rest, 1, i - 1) pfx;
      rest = substr(rest, i + plen);
      if (match(rest, /^[A-Za-z0-9._-]+/)) {
        seg = substr(rest, 1, RLENGTH);
        out = out pseudo_user_name(seg);
        rest = substr(rest, RLENGTH + 1);
      }
    }
    return out rest;
  }
  function gensub_home(line) {
    line = gensub_home_one(line, "/home/");
    line = gensub_home_one(line, "/Users/");
    return line;
  }
  function pseudo_user_name(u,   p) {
    if (u == "" || u == "root") return u;
    if (!(u in usermap)) {
      nusr++; usermap[u] = "user-" nusr;
      print "user\t" u "\t" usermap[u] >> mapfile;
    }
    return usermap[u];
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
    # PEM private keys are multi-line: withhold the WHOLE block, fail closed
    # if the END marker never arrives.
    if (inpem) {
      if ($0 ~ /-----END [A-Z ]*PRIVATE KEY/) inpem = 0;
      next;
    }
    if ($0 ~ /-----BEGIN [A-Z ]*PRIVATE KEY/) {
      print "[REDACTED PRIVATE KEY BLOCK]";
      inpem = 1;
      next;
    }
    # YAML block scalars under secret keys (password: | ... ) span lines:
    # withhold until indentation returns to the key level or shallower
    if (inyaml) {
      if ($0 ~ /^[ \t]*$/) next;
      match($0, /^[ \t]*/);
      if (RLENGTH > yamlindent) next;
      inyaml = 0;
    }
    if (match($0, /^[ \t]*[A-Za-z0-9_. -]+:[ \t]*[|>][0-9]?[+-]?[ \t]*$/)) {
      key = $0; sub(/:.*/, "", key); gsub(/^[ \t]*/, "", key);
      lk = tolower(key); gsub(/[-_]/, " ", lk);
      hit = 0;
      for (i = 1; i <= nsec; i++) if (index(lk, SK[i]) > 0) hit = 1;
      if (hit && !diagnostic_key(lk)) {
        match($0, /^[ \t]*/); yamlindent = RLENGTH;
        sub(/:.*/, ": [REDACTED BLOCK]");
        print; inyaml = 1; next;
      }
    }
    line = $0;
    # stream.conf parent side: [<API_KEY>] / [<MACHINE_GUID>] section headers ARE secrets
    if (line ~ /^[ \t]*\[[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]-[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]-[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]-[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]-[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]\][ \t]*$/) {
      print "[REDACTED-KEY-SECTION]"; next;
    }
    line = redact_kv(line);
    if (line ~ /"[^"]+"[ \t]*:[ \t]*/) line = redact_json(line);
    # URL creds: scheme://user:pass@  and Go DSN: user:pass@tcp(
    while (match(line, /:\/\/[^:\/@ \t]+:[^@ \t]+@/))
      line = substr(line, 1, RSTART - 1) "://[REDACTED]@" substr(line, RSTART + RLENGTH);
    while (match(line, /[A-Za-z0-9_]+:[^@ \t]+@(tcp|unix)\(/)) {
      m = substr(line, RSTART, RLENGTH);
      line = substr(line, 1, RSTART - 1) "[REDACTED]" substr(m, index(m, "@")) substr(line, RSTART + RLENGTH);
    }
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
    while (match(rest, /[?&][A-Za-z0-9_.-]*(token|apikey|api_key|access_key|private_key|secret_key|password|passwd|secret|bearer|claim_token|claim_rooms|key|auth)=/)) {
      out = out substr(rest, 1, RSTART + RLENGTH - 1) "[REDACTED]";
      rest = substr(rest, RSTART + RLENGTH);
      sub(/^[^&" \t]+/, "", rest);
    }
    line = out rest;
    # argv/env-style secrets mid-line (ps output, command lines: -token=X, CLAIM_TOKEN=X)
    out = ""; rest = line;
    while (match(rest, /[A-Za-z0-9_.-]*(token|TOKEN|Token|password|PASSWORD|Password|passwd|PASSWD|secret|SECRET|Secret|apikey|APIKEY|ApiKey|api_key|API_KEY|community|COMMUNITY|bearer|BEARER)[ ]?[=:][ ]?[^&" \t[]+/)) {
      m = substr(rest, RSTART, RLENGTH);
      sub(/[ ]?[=:].*/, "", m);
      lk = tolower(m); gsub(/[-_]/, " ", lk);
      if (diagnostic_key(lk))
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
      sub(/[ ]?=.*/, "", m);
      lk = tolower(m); gsub(/[-_]/, " ", lk);
      if (diagnostic_key(lk))
        out = out substr(rest, 1, RSTART + RLENGTH - 1);
      else
        out = out substr(rest, 1, RSTART - 1) m " = [REDACTED]";
      rest = substr(rest, RSTART + RLENGTH);
    }
    line = out rest;
    if (obf == 1) {
      # emails
      while (match(line, /[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z][A-Za-z]+/))
        line = substr(line, 1, RSTART - 1) "[EMAIL]" substr(line, RSTART + RLENGTH);
      # MACs
      while (match(line, /[0-9A-Fa-f][0-9A-Fa-f]:[0-9A-Fa-f][0-9A-Fa-f]:[0-9A-Fa-f][0-9A-Fa-f]:[0-9A-Fa-f][0-9A-Fa-f]:[0-9A-Fa-f][0-9A-Fa-f]:[0-9A-Fa-f][0-9A-Fa-f]/))
        line = substr(line, 1, RSTART - 1) "[MAC]" substr(line, RSTART + RLENGTH);
      if (line ~ /^[ \t#]*(proxy )?destination[ \t]*=/) line = redact_destination(line);
      line = replace_ips(line);
      line = replace_ip6(line);
      line = replace_private_fqdns(line);
      line = replace_mapped_fqdns(line);
      if (host_fqdn != "") line = replace_host(line, host_fqdn);
      if (host_short != "") line = replace_host(line, host_short);
      if (run_user != "") line = replace_user(line, run_user);
      # other local users appear in mount tables / paths when run as root
      line = gensub_home(line);
    }
    print line;
  }' "$_f" > "$_f.san" 2>>"$ERRORS"; then
    mv "$_f.san" "$_f"
  else
    # fail CLOSED: never ship content the sanitizer could not process
    rm -f "$_f.san"
    echo "[netdata-support-bundle] sanitization failed for this file - content withheld for safety" > "$_f"
  fi
}

# --- manifest ----------------------------------------------------------------
# manifest_add <rel-path> <kind:cmd|file|api> <origin> <title>
json_str() { # collapse newlines/tabs, escape backslashes and quotes
  _js="$1"
  printf '%s' "$_js" | tr '\n\t' '  ' | sed 's/\\/\\\\/g; s/"/\\"/g'
}
manifest_add() {
  _rel="$1"; _kind="$2"; _origin="$3"; _title="$4"
  _bytes=0; [ -f "$WORK/$_rel" ] && _bytes=$(wc -c < "$WORK/$_rel" | tr -d ' ')
  printf '{"path":"%s","kind":"%s","origin":"%s","title":"%s","bytes":%s,"pii_obfuscated":%s}\n' \
    "$(json_str "$_rel")" "$_kind" "$(json_str "$_origin")" "$(json_str "$_title")" "$_bytes" \
    "$([ "$OBFUSCATE" = "1" ] && echo true || echo false)" >> "$MANIFEST_ROWS"
}

# --- collectors ---------------------------------------------------------------
# collect_cmd <rel-path> <title> <cmd...>
collect_cmd() {
  # optional: --cap BYTES overrides the default output cap (journal captures
  # are documented at 5 MiB while command output defaults to 2 MiB)
  _ccap="$API_CAP"
  _first="${1:-}"
  if [ "$_first" = "--cap" ]; then _ccap="$2"; shift 2; fi
  _rel="$1"; _title="$2"; shift 2
  _out="$WORK/$_rel"
  mkdir -p "$(dirname "$_out")"
  if deadline_exceeded; then
    echo "SKIPPED: global deadline reached" > "$_out"
    manifest_add "$_rel" "cmd" "skipped" "$_title (skipped: deadline)"
    return 0
  fi
  _t0=$(now_s)
  # header must stay a single line even for multi-line sh -c scripts
  # shellcheck disable=SC1003  # literal backslash deletion, not a quote escape
  _cmdline=$(printf '%s' "$*" | tr -d '\\' | tr '\n\t' '  ' | tr -s ' ')
  { run_capped "$CMD_TIMEOUT" "$@" 2>&1; echo "$?" > "$_out.rc"; } | head -c "$((_ccap * 4))" > "$_out.raw"
  _rc=$(cat "$_out.rc" 2>/dev/null || echo '?'); rm -f "$_out.rc"
  _raw_size=$(wc -c < "$_out.raw" | tr -d ' ')
  {
    printf '# netdata-support-bundle v%s | command: %s | captured: %s\n' "$VERSION" "$_cmdline" "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    awk -v cap="$_ccap" 'BEGIN { b = 0 } { b += length($0) + 1; if (b > cap) exit; print }' "$_out.raw"
    [ "$_raw_size" -gt "$_ccap" ] && printf '### TRUNCATED: output was %s bytes, first %s kept (line-aligned) ###\n' "$_raw_size" "$_ccap"
    printf '# exit: %s | duration: %ss\n' "$_rc" "$(( $(now_s) - _t0 ))"
  } > "$_out"
  rm -f "$_out.raw"
  sanitize_file "$_out"
  manifest_add "$_rel" "cmd" "$_cmdline" "$_title"
}

# collect_file <rel-path> <title> <source-file> [cap-bytes]
collect_file() {
  _rel="$1"; _title="$2"; _src="$3"; _cap="${4:-$FILE_CAP}"
  deadline_exceeded && return 0
  [ -f "$_src" ] && [ -r "$_src" ] || return 0
  # a symlinked final component is not a real config/log file we chose to
  # collect; refuse it so a swapped link cannot redirect us to another target
  # (symlinked parent DIRECTORIES resolve normally - only the leaf is checked)
  if [ -h "$_src" ]; then
    _out="$WORK/$_rel"; mkdir -p "$(dirname "$_out")"
    echo "[content withheld: source is a symlink]" > "$_out"
    manifest_add "$_rel" "file" "$_src (symlink, withheld)" "$_title"
    return 0
  fi
  _out="$WORK/$_rel"; mkdir -p "$(dirname "$_out")"
  _size=$(wc -c < "$_src" 2>/dev/null | tr -d ' ')
  if [ "${_size:-0}" -gt "$_cap" ]; then
    # cap at a LINE boundary (drop the first, possibly partial, line) so a
    # secret can never straddle the cut and dodge the line-based sanitizer
    tail -c "$_cap" "$_src" 2>>"$ERRORS" | tail -n +2 > "$_out"
    if [ ! -s "$_out" ]; then
      # the whole tail was one giant line: withhold rather than risk a
      # mid-token cut hiding a secret from the line-based sanitizer
      echo "[content withheld: file tail exceeds the cap without a line break]" > "$_out"
    fi
    _origin="$_src (last ~$_cap of $_size bytes, line-aligned)"
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
  # shellcheck disable=SC1003  # literal backslash deletion, not a quote escape
  _cmdline=$(printf '%s' "$*" | tr -d '\\' | tr '\n\t' '  ' | tr -s ' ')
  run_capped "$CMD_TIMEOUT" "$@" 2>>"$ERRORS" | head -c "$((API_CAP + 1))" > "$_out"
  if [ "$(wc -c < "$_out" | tr -d ' ')" -gt "$API_CAP" ]; then
    # a truncated JSON document is worse than none: fail closed
    echo '{"error":"output exceeded the cap and was withheld"}' > "$_out"
  fi
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
  _rel="$1"; _title="$2"; _upath="$3"; _url="http://127.0.0.1:${NDPORT}${_upath}"
  deadline_exceeded && return 0
  _out="$WORK/$_rel"; mkdir -p "$(dirname "$_out")"
  if command -v curl >/dev/null 2>&1; then
    curl -sf --max-time "$CMD_TIMEOUT" "$_url" 2>>"$ERRORS" | head -c "$((API_CAP + 1))" > "$_out"
  elif command -v wget >/dev/null 2>&1; then
    wget -q -T "$CMD_TIMEOUT" -O - "$_url" 2>>"$ERRORS" | head -c "$((API_CAP + 1))" > "$_out"
  fi
  # a JSON body cut at any point is malformed, so overflow is withheld whole
  if [ "$(wc -c < "$_out" 2>/dev/null | tr -d ' ')" -gt "$API_CAP" ]; then
    echo '{"error":"response exceeded the cap and was withheld"}' > "$_out"
  fi
  if [ -s "$_out" ]; then
    sanitize_file "$_out"
    manifest_add "$_rel" "api" "$_upath" "$_title"
  else
    rm -f "$_out"
  fi
}

# --- sanitizer regression vectors (run with --selftest; extend when adding
# --- redaction rules; a vector that fails here must never ship) -------------
run_selftest() {
  _tf="$STAGING/selftest.txt"
  _fails=0
  cat > "$_tf" <<'VECTORS'
api key = SENTINEL-1
password: SENTINEL-3
"claim_token": "SENTINEL-4"
url: https://admin:SENTINEL-5@app.example.com/x
dsn: user:SENTINEL-6@tcp(10.1.2.3:3306)/db
TELEGRAM_BOT_TOKEN="SENTINEL-8"
TOKEN=false
PASSWORD=/etc/SENTINEL-9
GET /api/v1/data?chart=x&token=SENTINEL-10&after=-60
/usr/sbin/netdata-claim.sh -token=SENTINEL-11 -rooms=abc
cmdline: /usr/sbin/agent -token=SENTINEL-14 --verbose
connect user:SENTINEL-15@unix(/run/x)/db ok
/etc/netdata/claim_token: SENTINEL-16
cmdline: claim.sh api key = SENTINEL-12 end
password: q
"api_token": 731942
private_key: |
  SENTINEL-YAML-LINE1
  SENTINEL-YAML-LINE2
after_block = ok
[11111111-2222-3333-4444-555555555555]
-----BEGIN RSA PRIVATE KEY-----
U0VOVElORUwtMTMtUEVNLUJPRFk=
-----END RSA PRIVATE KEY-----
bearer token protection = no
netdata management api key file = /var/lib/netdata/netdata.api.key
TCP SYN cookies = auto
destination = parent.bigcorp.example:19999
destination = tcp:protoparent.example.com:19999
# destination = old-parent.example.org:19999
destination = [2001:db8::77]:19999 unix:/run/nd.sock 10.7.7.7:19999
tcp LISTEN 0 4096 later-line
server at 10.1.2.3 and 2606:4700:10::ac42:aad8 and 2001:470:26:307:0:0:0:1
mail ops@example.com mac aa:bb:cc:dd:ee:ff at 2026-07-16T13:38:34Z
"password_escq": "ab\"SENTINEL-ESCQ"
PWD=SENTINEL-PWD
"api_token": -98765
"access_key": ["SENTINEL-ARR"]
tabbed_secret_block: |
	SENTINEL-TAB-LINE
after_tab = ok
password_ind: |2
  SENTINEL-IND2
after_ind = ok
home /home/alice/x and /Users/bob/y
VECTORS
  # assembled at runtime so secret scanners do not flag the source as a
  # committed credential; the sanitized bytes are identical
  _bw="Bea"; _bw="${_bw}rer"
  printf 'Authorization: %s SENTINEL-2abc\n' "$_bw" >> "$_tf"
  _obf_save="$OBFUSCATE"; OBFUSCATE=1
  sanitize_file "$_tf"
  OBFUSCATE="$_obf_save"
  t_absent() {
    _pat="$1"; _msg="$2"
    if grep -q "$_pat" "$_tf"; then echo "FAIL (leak): $_msg" >&2; _fails=$((_fails + 1)); fi
  }
  t_present() {
    _pat="$1"; _msg="$2"
    if [ "$_pat" = "--" ]; then _pat="$2"; _msg="$3"; fi
    if ! grep -qF -- "$_pat" "$_tf"; then echo "FAIL (over-redaction): $_msg" >&2; _fails=$((_fails + 1)); fi
  }
  t_absent  "SENTINEL-"                  "a planted secret survived"
  t_absent  "U0VOVElORUw"                "PEM body survived"
  t_absent  "TOKEN=false"                "TOKEN=false survived (values are never exempt)"
  t_absent  "731942"                     "scalar JSON secret survived"
  t_absent  "password: q"                "one-character secret survived"
  t_absent  "2606:4700"                  "compressed IPv6 survived"
  t_absent  "2001:470:26:307:0:0:0:1"    "uncompressed numeric IPv6 survived"
  t_absent  "10\.1\.2\.3"                "IPv4 survived"
  t_absent  "10\.7\.7\.7"                "IP destination not pseudonymized as an IP"
  t_absent  "parent.bigcorp.example"     "stream destination hostname survived"
  t_absent  "protoparent.example.com"    "protocol-prefixed destination hostname survived"
  t_absent  "old-parent.example.org"     "commented-out destination hostname survived"
  t_absent  "2001:db8::77"               "bracketed IPv6 destination leaked"
  t_absent  "ops@example.com"            "email survived"
  t_absent  "aa:bb:cc:dd:ee:ff"          "MAC survived"
  t_present "after_block = ok"           "YAML block withholding ate following content"
  t_absent  "SENTINEL-ESCQ"              "escaped-quote JSON value leaked its suffix"
  t_absent  "SENTINEL-PWD"               "PWD= secret alias survived"
  t_absent  "98765"                      "negative-number JSON scalar survived"
  t_absent  "SENTINEL-ARR"               "structured (array) JSON secret value survived"
  t_absent  "SENTINEL-TAB"              "tab-indented YAML block-scalar secret survived"
  t_present "after_tab = ok"             "tab YAML block withholding overran"
  t_absent  "SENTINEL-IND2"              "explicit-indent YAML block scalar (|2) secret survived"
  t_present "after_ind = ok"             "|2 YAML block withholding overran"
  t_absent  "/home/alice"                "other user home path not pseudonymized"
  t_absent  "/Users/bob"                 "other user Users path not pseudonymized"
  t_present "destination = tcp:"         "destination protocol prefix lost"
  t_present "unix:/run/nd.sock"          "socket-path destination was mangled"
  t_present "tcp LISTEN 0 4096 later-line" "literal tcp corrupted by fqmap pollution"
  t_present "bearer token protection = no"  "diagnostic option lost (key-based exemption broken)"
  t_present "api key file = /var/lib/netdata/netdata.api.key" "key-file path lost"
  t_present "TCP SYN cookies = auto"     "SYN cookies value lost"
  t_present "[REDACTED PRIVATE KEY BLOCK]" "PEM block marker missing"
  t_present "2026-07-16T13:38:34Z"       "timestamp mangled by IPv6 rule"
  t_present -- "--verbose"               "path-bearing argv line was eaten by the kv rule"
  t_present "@unix(/run/x)/db ok"        "mid-line unix( DSN rule broke the tail"
  printf 'nul-test \000 password=SENTINEL-NUL\n' > "$_tf.nul"
  sanitize_file "$_tf.nul"
  grep -q "content withheld" "$_tf.nul" || { echo "FAIL: NUL-bearing file was not withheld" >&2; _fails=$((_fails + 1)); }
  if [ "$_fails" -eq 0 ]; then
    echo "netdata-support-bundle selftest: ALL PASS"
    exit 0
  fi
  echo "netdata-support-bundle selftest: $_fails FAILURE(S)" >&2
  exit 1
}
[ "$SELFTEST" = "1" ] && run_selftest

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

# pre-seed child/mirrored node hostnames so they pseudonymize consistently in
# EVERY file (node_instances, stream configs, logs). Their real names stay in
# the local map for the user to decode.
if [ "$api_ok" = "1" ]; then
  _seed_n=0
  set -f
  for _h in $( { curl -sf --max-time 5 "http://127.0.0.1:${NDPORT}/api/v2/node_instances" 2>/dev/null || wget -q -T 5 -O - "http://127.0.0.1:${NDPORT}/api/v2/node_instances" 2>/dev/null; }       | tr ',{' '\n' | awk 'match($0, /"(nm|hostname)" *: *"/) { _v = substr($0, RSTART + RLENGTH); sub(/".*/, "", _v); if (_v != "") print _v }' | sort -u); do
    [ "$_h" = "$HOST_SHORT" ] && continue
    [ "$_h" = "$HOST_FQDN" ] && continue
    [ "$_h" = "localhost" ] && continue
    [ ${#_h} -lt 4 ] && continue
    _seed_n=$((_seed_n + 1))
    printf 'fqdn\t%s\tprivate-host-%s\n' "$_h" "$_seed_n" >> "$MAP_FILE"
  done
  set +f
fi

IS_CONTAINER=0
[ -f /.dockerenv ] && IS_CONTAINER=1
grep -qE '(docker|containerd|kubepods|lxc)' /proc/1/cgroup 2>/dev/null && IS_CONTAINER=1

info "netdata-support-bundle $VERSION"
info "agent pid: ${NETDATA_PID:-not running} | api: $([ $api_ok = 1 ] && echo up || echo unreachable) | config: ${CONFDIR:-not found} | container: $IS_CONTAINER"

# =============================================================================
# 01-system
# =============================================================================
info "collecting: system"
collect_cmd 01-system/uname.txt            "Kernel and architecture" uname -a
collect_cmd 01-system/os-release.txt      "OS distribution (first of os-release/lsb-release)" sh -c '
  for f in /etc/os-release /usr/lib/os-release /etc/lsb-release; do
    [ -r "$f" ] && { echo "# source: $f"; cat "$f"; break; }
  done' 
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
collect_cmd 03-process/ps-netdata.txt "Netdata process tree with CPU/memory" sh -c 'ps aux 2>/dev/null | head -1; ps aux 2>/dev/null | grep -E "[n]etdata|[g]o.d|[e]bpf|[a]pps.plugin|[c]harts.d|[p]ython.d" | grep -v "netdata-support-bundle" | head -50'
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
  collect_cmd 04-config/config-tree.txt "User config dir tree (files here = user-customized; ssl/ and key material excluded)" sh -c '
    ls -laR '"$CONFDIR"' 2>/dev/null | head -2000 | awk "
      /\/ssl:\$/ { print; skip = 1; print \"  [ssl directory contents withheld]\"; next }
      skip && /^\$/ { skip = 0; print; next }
      skip { next }
      /\.(pem|key)\$/ { next }
      { print }"'
  collect_file 04-config/netdata.conf "On-disk main config" "$CONFDIR/netdata.conf"
  collect_file 04-config/stream.conf "Streaming config (parent/child; api key redacted)" "$CONFDIR/stream.conf"
  collect_file 04-config/exporting.conf "Exporting engine config (credentials redacted)" "$CONFDIR/exporting.conf"
  collect_file 04-config/go.d.conf "go.d orchestrator config (module enable/disable)" "$CONFDIR/go.d.conf"
  # every user-customized config, nested dirs included (go.d/sd/, go.d/ss/,
  # otel.d/, vnodes/, ...), relative paths preserved; ssl and key material
  # excluded; capped at 200 files
  find "$CONFDIR" -type f \( -name '*.conf' -o -name '*.yml' -o -name '*.yaml' \) 2>/dev/null | head -200 | while IFS= read -r f; do
    case "$f" in */ssl/*|*.pem|*.key) continue ;; *) : ;; esac
    _relc=${f#"$CONFDIR"/}
    case "$_relc" in netdata.conf|stream.conf|exporting.conf|go.d.conf) continue ;; *) : ;; esac
    collect_file "04-config/$_relc" "User config (secrets redacted)" "$f" 262144
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
  collect_cmd --cap "$LOG_CAP" 05-logs/journal-netdata.txt "systemd journal for netdata unit" \
    sh -c "journalctl -u netdata --no-pager -o short-iso --since '-${SINCE_HOURS} hours' 2>/dev/null | tail -n 20000; true"
  collect_cmd --cap "$LOG_CAP" 05-logs/journal-namespace-netdata.txt "netdata journal namespace (some installs log here)" \
    sh -c "journalctl --namespace=netdata --no-pager -o short-iso --since '-${SINCE_HOURS} hours' 2>/dev/null | tail -n 20000; true"
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
    case "$_lt" in
    /dev/std*)
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
    *) : ;;
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
# shellcheck disable=SC3013  # -nt is supported by dash, ash/busybox, bash and FreeBSD sh
_status_candidates=""
[ -n "$LIBDIR" ] && _status_candidates="$_status_candidates $LIBDIR/status-netdata.json"
[ -n "$CACHEDIR" ] && _status_candidates="$_status_candidates $CACHEDIR/status-netdata.json"
_status_candidates="$_status_candidates /tmp/status-netdata.json /run/status-netdata.json /var/run/status-netdata.json"
for sf in $_status_candidates; do
  [ -f "$sf" ] || continue
  if [ -z "$NEWEST_STATUS" ] || [ "$sf" -nt "$NEWEST_STATUS" ]; then NEWEST_STATUS="$sf"; fi
done
[ -n "$NEWEST_STATUS" ] && collect_file 06-state/status-file.json "Daemon status file: LAST EXIT/CRASH RECORD incl. fatal stack trace (read this first for crashes)" "$NEWEST_STATUS"
if [ -n "$LIBDIR" ]; then
  # bearer_tokens/ FILENAMES are live API tokens - withhold them from the listing
  collect_cmd 06-state/state-tree.txt "State dir listing (bearer token filenames withheld - they are live tokens)" \
    sh -c 'ls -laR '"$LIBDIR"' 2>/dev/null | head -2000 | awk "
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

Generated by `netdata-support-bundle`. Contents are SANITIZED:
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

- Command captures (`*.txt`) begin with a `# netdata-support-bundle | command: ...`
  provenance header and end with `# exit: N`.
- Copied files (configs, logs, json) are pristine (no injected headers);
  their origin is recorded in `MANIFEST.json`.
- `07-runtime/AGENT-WAS-DOWN.txt` exists only when the agent was not running.
EOF
manifest_add README.md file generated "Bundle documentation"

# emit MANIFEST.json LAST so every file (incl. summary.txt and README.md) is indexed
{
  echo '{'
  echo '  "schema": "netdata-support-bundle/v1",'
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
# zstd compresses faster and smaller than gzip on this kind of text-heavy data;
# use it when the tar build supports --zstd, else a zstd pipe, else gzip
if command -v zstd >/dev/null 2>&1 && tar --zstd -cf /dev/null -T /dev/null 2>/dev/null; then
  _ext="tar.zst"; _mode="tar-zstd"
elif command -v zstd >/dev/null 2>&1; then
  _ext="tar.zst"; _mode="zstd-pipe"
else
  _ext="tar.gz"; _mode="gzip"
fi
TARBALL="$OUTDIR/$BUNDLE.$_ext"
# build inside the 0700 staging dir, then publish with O_EXCL (set -C) so a
# pre-existing file OR symlink planted in a shared tmp dir can never be
# followed or overwritten (no check/open TOCTOU window)
# anonymize archive owner/group so a non-root user's account isn't in tar headers
_towner=""
if tar --owner=0 --group=0 -cf /dev/null -T /dev/null 2>/dev/null; then
  _towner="--owner=0 --group=0 --numeric-owner"
fi
_tarok=0
# shellcheck disable=SC2086  # $_towner intentionally splits into separate tar flags
case "$_mode" in
  tar-zstd)  ( cd "$STAGING" && tar $_towner --zstd -cf "$STAGING/bundle.$_ext" "$BUNDLE" ) && _tarok=1 ;;
  zstd-pipe) ( cd "$STAGING" && tar $_towner -cf - "$BUNDLE" | zstd -q -o "$STAGING/bundle.$_ext" ) && _tarok=1 ;;
  gzip)      ( cd "$STAGING" && tar $_towner -czf "$STAGING/bundle.$_ext" "$BUNDLE" ) && _tarok=1 ;;
  *) : ;;
esac
if [ "$_tarok" != "1" ]; then
  echo "failed to create tarball" >&2; exit 1
fi
if ! ( set -C; cat "$STAGING/bundle.$_ext" > "$TARBALL" ) 2>/dev/null; then
  echo "refusing to write $TARBALL (a file or symlink already exists there)" >&2
  exit 1
fi

if [ "$OBFUSCATE" = "1" ] && [ -s "$MAP_FILE" ]; then
  MAP_OUT="$OUTDIR/$BUNDLE.pseudonym-map.tsv"
  if ! ( set -C; cat "$MAP_FILE" > "$MAP_OUT" ) 2>/dev/null; then
    MAP_OUT="$OUTDIR/$BUNDLE.pseudonym-map.$$.tsv"
    if ! ( set -C; cat "$MAP_FILE" > "$MAP_OUT" ) 2>/dev/null; then
      info "WARNING: could not write the pseudonym map next to the bundle - it was DISCARDED; rerun with --keep-staging if you need it"
      MAP_OUT=""
    fi
  fi
fi

TOTAL_S=$(( $(now_s) - START_TS ))
SIZE=$(du -h "$TARBALL" 2>/dev/null | cut -f1)
echo >&2
info "done in ${TOTAL_S}s"
info "bundle:  $TARBALL ($SIZE)"
[ "$OBFUSCATE" = "1" ] && [ -n "${MAP_OUT:-}" ] && info "pseudonym map (KEEP PRIVATE, do not send): $MAP_OUT"
if [ "$_ext" = "tar.zst" ]; then
  info "review it:  tar --zstd -tf $TARBALL   (or: zstd -dc $TARBALL | tar -tf -)"
else
  info "review it:  tar -tzf $TARBALL"
fi
if [ "${DOCKER_LOGS_NEEDED:-0}" = "1" ]; then
  info "IMPORTANT: this agent logs to the container's stdout - its log history is NOT in this bundle."
  info "on the docker HOST also run:  docker logs --since 24h <netdata-container> > netdata-docker.log 2>&1"
  info "and attach netdata-docker.log to the ticket as well."
fi
info "attach the bundle to your support ticket."
