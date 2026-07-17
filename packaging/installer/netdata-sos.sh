#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later
#
# shellcheck disable=SC2016  # the sh -c and awk mini-programs in the collectors
# are intentionally single-quoted: they must reach the child shell/awk unexpanded.
#
# netdata-sos - collect a sanitized diagnostic bundle for Netdata support tickets.
#
# What is collected and WHY each item is included is documented in
# packaging/installer/SUPPORT-BUNDLE.md - read it before adding or changing
# a collection item, and keep it in sync with this script.
#
# Design contract:
#   - Works with or without a running agent, with or without root (degrades gracefully).
#   - Zero system impact: idle CPU/IO priority, per-command timeouts, global
#     deadline, size caps, read-only collection, and only requested artifacts.
#   - Secrets are ALWAYS redacted (non-optional). PII (IPs, MACs, emails, hostnames)
#     pseudonymized by default; --no-obfuscate disables PII pass only.
#   - Bundle is legible to humans AND AI agents: triage-ordered directories,
#     sanitized file copies, provenance in MANIFEST.json, summary and README.
#
# Usage: sudo sh netdata-sos.sh [options]
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
if [ -z "${ND_SOS_DEMOTED:-}" ]; then
  ND_SOS_DEMOTED=1; export ND_SOS_DEMOTED
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
GLOBAL_DEADLINE=240
LOG_CAP=5242880          # 5 MiB per log file
FILE_CAP=1048576         # 1 MiB per config/state file
API_CAP=2097152          # 2 MiB per API response
SANITIZE_INPUT_CAP=16777216 # 16 MiB raw window/stream guard before fail-closed withholding
PSEUDONYM_MAP_MAX=4096     # bound global correlation state and private sidecar size

while [ $# -gt 0 ]; do
  case "$1" in
    -o|--output) OUTDIR="$2"; shift 2 ;;
    --since) SINCE_HOURS="$2"; shift 2 ;;
    --timeout) CMD_TIMEOUT="$2"; shift 2 ;;
    --no-obfuscate) OBFUSCATE=0; shift ;;
    --keep-staging) KEEP_STAGING=1; shift ;;
    --selftest) SELFTEST=1; shift ;;
    -v|--version) echo "netdata-sos $VERSION"; exit 0 ;;
    -h|--help) sed -n '/^# netdata-sos - collect/,/^#   -h, --help/p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown option: $1" >&2; exit 1 ;;
  esac
done

case "$SINCE_HOURS" in *[!0-9]*|''|0) echo "--since must be a positive integer (hours)" >&2; exit 1 ;; *) : ;; esac
case "$CMD_TIMEOUT" in *[!0-9]*|''|0) echo "--timeout must be a positive integer (seconds)" >&2; exit 1 ;; *) : ;; esac

umask 077
START_TS=$(date +%s)
NOW=$(date -u +%Y%m%d-%H%M%S)
STAGING=$(mktemp -d "${TMPDIR:-/tmp}/netdata-sos.XXXXXX") || exit 1
_BUNDLE_NONCE=${STAGING##*.}
BUNDLE="netdata-sos-${NOW}-$$-${_BUNDLE_NONCE}"
WORK="$STAGING/$BUNDLE"
mkdir -p "$WORK"
MAP_FILE="$STAGING/map.tsv"; : > "$MAP_FILE"
MANIFEST_ROWS="$STAGING/manifest.rows"; : > "$MANIFEST_ROWS"
ERRORS="$STAGING/errors.txt"; : > "$ERRORS"
PATH_FILTER="$STAGING/print-safe-paths.sh"
cat > "$PATH_FILTER" <<'EOF'
#!/bin/sh
newline='
'
for path do
  case "$path" in *"$newline"*) continue ;; esac
  printf '%s\n' "$path"
done
EOF

cleanup() { [ "$KEEP_STAGING" = "1" ] || rm -rf "$STAGING"; }
trap cleanup EXIT

info() { printf ' [*] %s\n' "$*" >&2; }
now_s() { date +%s; }
deadline_exceeded() { [ $(( $(now_s) - START_TS )) -ge "$GLOBAL_DEADLINE" ]; }

proc_process_table() {
  for _tree_stat in /proc/[0-9]*/stat; do
    [ -r "$_tree_stat" ] || continue
    IFS= read -r _tree_row < "$_tree_stat" || continue
    _tree_pid=${_tree_stat%/stat}; _tree_pid=${_tree_pid##*/}
    _tree_after_comm=${_tree_row##*) }
    # shellcheck disable=SC2086  # split the documented stat fields
    set -- $_tree_after_comm
    [ "${2:-}" ] && printf '%s %s\n' "$_tree_pid" "$2"
  done
}

# Return every descendant of a process. This works with procps, BSD/macOS ps,
# BusyBox ps, and Linux /proc when ps cannot expose parent PIDs.
descendant_pids() {
  _tree_root="$1"
  if ps -eo pid=,ppid= >/dev/null 2>&1; then
    ps -eo pid=,ppid=
  elif ps -axo pid=,ppid= >/dev/null 2>&1; then
    ps -axo pid=,ppid=
  elif [ -d /proc ]; then
    proc_process_table
  else
    return 0
  fi | awk -v root="$_tree_root" '
    { pid[NR] = $1; ppid[NR] = $2 }
    END {
      wanted[root] = 1
      for (pass = 1; pass <= NR; pass++)
        for (i = 1; i <= NR; i++)
          if (wanted[ppid[i]]) wanted[pid[i]] = 1
      for (i = 1; i <= NR; i++)
        if (pid[i] != root && wanted[pid[i]]) print pid[i]
    }'
}

# Reject links already present in a source path. The root/Administrator and
# Netdata service identities are trusted not to mutate source entries
# adversarially during collection; portable POSIX sh has no openat(2)-style,
# no-follow path walk. Keep that boundary explicit rather than presenting this
# snapshot check as an atomic open.
path_has_symlink() { # <path>; true when the file or any parent is a symlink
  _link_path="$1"
  while [ "$_link_path" != "/" ] && [ "$_link_path" != "." ]; do
    [ -L "$_link_path" ] && return 0
    _link_parent=$(dirname "$_link_path")
    [ "$_link_parent" = "$_link_path" ] && break
    _link_path="$_link_parent"
  done
  return 1
}

terminate_tree() { # <root-pid> <isolated-process-group:0|1>
  _tree_root="$1"; _tree_group="${2:-0}"
  if [ "$_tree_group" = "1" ]; then
    # The collector was started in its own session, so a process-group signal
    # also reaches descendants that fork while handling TERM.
    kill -TERM -"$_tree_root" 2>/dev/null || true
    sleep 1
    kill -KILL -"$_tree_root" 2>/dev/null || true
    return
  fi
  # Portable fallback for platforms without setsid: freeze the root first so
  # it cannot fork from a signal handler, then repeatedly freeze descendants
  # until the tree is stable. SIGSTOP/SIGKILL cannot be trapped.
  _tree_children=""
  _tree_round=0
  kill -STOP "$_tree_root" 2>/dev/null || true
  while [ "$_tree_round" -lt 4 ]; do
    _tree_children="$_tree_children $(descendant_pids "$_tree_root" | tr '\n' ' ')"
    # shellcheck disable=SC2086  # intentionally expand validated numeric PIDs
    kill -STOP $_tree_children 2>/dev/null || true
    _tree_round=$((_tree_round + 1))
  done
  # shellcheck disable=SC2086  # intentionally expand validated numeric PIDs
  kill -KILL "$_tree_root" $_tree_children 2>/dev/null || true
}

ACTIVE_CMD_PID=""
on_signal() {
  [ -z "$ACTIVE_CMD_PID" ] || terminate_tree "$ACTIVE_CMD_PID" "${ACTIVE_CMD_GROUP:-0}"
  cleanup
  trap - EXIT
  exit 130
}
trap on_signal INT TERM

run_capped_impl() { # <seconds> <stdin-file> <cmd...>
  _t="$1"; _command_input="$2"; shift 2
  _timeout_marker="$STAGING/timeout.$$.$(now_s)"
  rm -f "$_timeout_marker"
  _isolated_group=0
  # sanitize_stream is a shell function, not an executable. Keep its explicit
  # stdin path but use the portable tree fallback for that invocation.
  if [ "${ND_SOS_DISABLE_SETSID:-0}" != "1" ] && [ "${1:-}" != "sanitize_stream" ] &&
     command -v setsid >/dev/null 2>&1; then
    setsid "$@" < "$_command_input" &
    _isolated_group=1
  else
    "$@" < "$_command_input" &
  fi
  _cmdpid=$!
  ACTIVE_CMD_PID="$_cmdpid"
  ACTIVE_CMD_GROUP="$_isolated_group"
  (
    _i=0
    while [ "$_i" -lt "$_t" ]; do
      sleep 1
      kill -0 "$_cmdpid" 2>/dev/null || exit 0
      _i=$((_i + 1))
    done
    : > "$_timeout_marker"
    terminate_tree "$_cmdpid" "$_isolated_group"
  ) &
  _wdpid=$!
  wait "$_cmdpid"
  _rc=$?
  ACTIVE_CMD_PID=""
  ACTIVE_CMD_GROUP=""
  if [ -f "$_timeout_marker" ]; then
    wait "$_wdpid" 2>/dev/null
    rm -f "$_timeout_marker"
    return 124
  fi
  _watchdog_children=$(descendant_pids "$_wdpid" | tr '\n' ' ')
  # shellcheck disable=SC2086  # validated numeric PID list
  kill -KILL "$_wdpid" $_watchdog_children 2>/dev/null || true
  wait "$_wdpid" 2>/dev/null || true
  return "$_rc"
}

run_capped() { # run_capped <seconds> <cmd...>; collectors never inherit script stdin
  _run_timeout="$1"; shift
  run_capped_impl "$_run_timeout" /dev/null "$@"
}

run_capped_input() { # run_capped_input <seconds> <input-file> <cmd...>
  _run_timeout="$1"; _run_input="$2"; shift 2
  run_capped_impl "$_run_timeout" "$_run_input" "$@"
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

sanitize_stream() {
  awk -v obf="$OBFUSCATE" -v mapfile="$MAP_FILE" \
      -v host_short="$HOST_SHORT" -v host_fqdn="$HOST_FQDN" -v run_user="$RUN_USER" \
      -v mapmax="$PSEUDONYM_MAP_MAX" '
  BEGIN {
    nsec = split("api key,apikey,apitoken,token,accesstoken,authtoken,claimtoken,refreshtoken,sessiontoken,password,passwd,pass,pwd,pat,key,secret,clientsecret,clientpassword,community,bearer,webhook,webhookurl,license key,licensekey,auth,credential,credentials,cookie,passphrase,proxy user,proxy pass,proxypassword,username,dsn,private key,privatekey,access key,accesskey,session,recipient,account sid,accountsid,priv key", SK, ",");
    nip = 0;
    while ((getline line < mapfile) > 0) {
      split(line, a, "\t");
      nmap++;
      if (a[1] == "ip") { ipmap[a[2]] = a[3]; nip++; }
      else if (a[1] == "host") hostmap[tolower(a[2])] = a[3];
      else if (a[1] == "user") usermap[a[2]] = a[3];
      else if (a[1] == "ip6") { ip6map[a[2]] = a[3]; nip6++; }
      else if (a[1] == "fqdn") { fqmap[tolower(a[2])] = a[3]; nfq++; }
    }
    close(mapfile);
  }
  function normalize_key(key,   expanded, i, c, prev, nxt, k) {
    # Split camelCase and acronym-to-word boundaries before punctuation
    # normalization (dbPassword, APISecret, githubToken).
    expanded = "";
    for (i = 1; i <= length(key); i++) {
      c = substr(key, i, 1);
      prev = (i > 1) ? substr(key, i - 1, 1) : "";
      nxt = substr(key, i + 1, 1);
      if (i > 1 && c ~ /[A-Z]/ &&
          (prev ~ /[a-z0-9]/ || (prev ~ /[A-Z]/ && nxt ~ /[a-z]/)))
        expanded = expanded " ";
      expanded = expanded c;
    }
    k = tolower(expanded);
    gsub(/[-_.]/, " ", k);
    gsub(/[ \t]+/, " ", k);
    gsub(/^[ \t#]+|[ \t]+$/, "", k);
    return k;
  }
  function hex_digit(c) {
    c = tolower(c);
    return index("0123456789abcdef", c) - 1;
  }
  function decode_json_key(key,   out, rest, m, code) {
    out = ""; rest = key;
    while (match(rest, /\\u00[0-7][0-9A-Fa-f]/)) {
      m = substr(rest, RSTART, RLENGTH);
      code = hex_digit(substr(m, 5, 1)) * 16 + hex_digit(substr(m, 6, 1));
      out = out substr(rest, 1, RSTART - 1) sprintf("%c", code);
      rest = substr(rest, RSTART + RLENGTH);
    }
    return out rest;
  }
  function secret_key(key,   k, padded, i) {
    k = normalize_key(decode_json_key(key));
    padded = " " k " ";
    for (i = 1; i <= nsec; i++)
      if (index(padded, " " SK[i] " ") > 0) return 1;
    return 0;
  }
  function diagnostic_key(lk) {
    # exemptions are decided by the KEY, never the value (a secret can be any
    # string, incl. "false" or a path). Keys ENDING in these words describe
    # secrets or toggles rather than being secrets: "bearer token protection",
    # "netdata management api key file", "TCP SYN cookies".
    lk = normalize_key(lk);
    if (lk ~ /(^| )(file|path|dir|directory|protection|support|mode|level|port|timeout|cookies|secure|log|size|options|format|type)$/) return 1;
    return 0;
  }
  function redact_kv(line,   i, k, pos, posc, pose, key, lk, value, block, indent) {
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
      if (length(key) > 64 || key !~ /^[A-Za-z0-9]/ || key ~ /["`;|()\/]/ || key ~ /[ \t]-/) return line;
      lk = normalize_key(key);
      if (diagnostic_key(lk)) return line;
      value = substr(line, pos + 1);
      gsub(/^[ \t]+|[ \t]+$/, "", value);
      # YAML permits indentation/chomping indicators and anchors/tags before a
      # block scalar (|2-, &name |, !tag >+). Any such secret value requires
      # withholding its indented continuation.
      block = (value ~ /(^|[ \t])[|>][0-9+-]*([ \t]|$)/);
      if (secret_key(lk) && (length(value) > 0 || block)) {
        if (block) {
          indent = match(line, /[^ \t]/) - 1;
          yaml_hold = 1; yaml_hold_indent = indent;
        }
        return substr(line, 1, pos) " [REDACTED]";
      }
    }
    return line;
  }
  function json_string_end(s, start,   i, c, escaped) {
    escaped = 0;
    for (i = start + 1; i <= length(s); i++) {
      c = substr(s, i, 1);
      if (escaped) escaped = 0;
      else if (c == "\\") escaped = 1;
      else if (c == "\"") return i;
    }
    return 0;
  }
  function json_skip_ws(s, start,   i, c) {
    for (i = start; i <= length(s); i++) {
      c = substr(s, i, 1);
      if (c != " " && c != "\t" && c != "\r" && c != "\n") return i;
    }
    return length(s) + 1;
  }
  function json_composite_reset(   i) {
    for (i in json_comp_stack) delete json_comp_stack[i];
    json_comp_depth = 0; json_comp_string = 0; json_comp_escape = 0;
    json_comp_forever = 0;
  }
  function json_composite_scan(s, start,   i, c, opener) {
    for (i = start; i <= length(s); i++) {
      c = substr(s, i, 1);
      if (json_comp_string) {
        if (json_comp_escape) json_comp_escape = 0;
        else if (c == "\\") json_comp_escape = 1;
        else if (c == "\"") json_comp_string = 0;
        continue;
      }
      if (c == "\"") { json_comp_string = 1; continue; }
      if (c == "{" || c == "[") {
        json_comp_depth++;
        json_comp_stack[json_comp_depth] = c;
        continue;
      }
      if (c == "}" || c == "]") {
        opener = json_comp_stack[json_comp_depth];
        if (json_comp_depth <= 0 || (c == "}" && opener != "{") ||
            (c == "]" && opener != "[")) {
          json_comp_forever = 1; json_comp_depth = 0;
          return 0;
        }
        delete json_comp_stack[json_comp_depth];
        json_comp_depth--;
        if (json_comp_depth == 0) return i;
      }
    }
    return 0;
  }
  function json_value_end(s, start,   opener, c, i) {
    opener = substr(s, start, 1);
    if (opener == "\"") return json_string_end(s, start);
    if (opener == "{" || opener == "[") {
      json_composite_reset();
      return json_composite_scan(s, start);
    }
    for (i = start; i <= length(s); i++) {
      c = substr(s, i, 1);
      if (c == "," || c == "}" || c == "]") return i - 1;
    }
    return length(s);
  }
  function json_start_withholding(s,   opener, e) {
    opener = substr(s, 1, 1);
    if (opener == "{" || opener == "[") {
      json_composite_reset();
      json_composite_scan(s, 1);
    }
    else if (opener == "\"") {
      e = json_string_end(s, 1);
      if (!e) json_hold_top_string = 1;
    }
  }
  function redact_json(line,   out, emit, scan, q, qe, colon, key, vs, ve) {
    # A quote-aware scanner handles escaped strings, scalar values, and nested
    # values. If a secret value is malformed, withhold the remainder of the
    # line instead of emitting a potentially sensitive fragment.
    out = ""; emit = 1; scan = 1;
    while (scan <= length(line)) {
      q = index(substr(line, scan), "\"");
      if (!q) break;
      q += scan - 1;
      qe = json_string_end(line, q);
      if (!qe) break;
      colon = json_skip_ws(line, qe + 1);
      if (substr(line, colon, 1) != ":") { scan = qe + 1; continue; }
      key = substr(line, q + 1, qe - q - 1);
      if (!secret_key(key) || diagnostic_key(key)) { scan = colon + 1; continue; }
      vs = json_skip_ws(line, colon + 1);
      if (vs > length(line)) {
        out = out substr(line, emit, colon - emit + 1) " \"[REDACTED]\"";
        json_hold_pending = 1;
        return out;
      }
      ve = json_value_end(line, vs);
      out = out substr(line, emit, vs - emit) "\"[REDACTED]\"";
      if (!ve) {
        json_start_withholding(substr(line, vs));
        return out;
      }
      emit = ve + 1;
      scan = emit;
    }
    return out substr(line, emit);
  }
  function pseudo_ip(ip,   p) {
    if (ip ~ /^127\./ || ip == "0.0.0.0" || ip ~ /^255\./) return ip;
    if (!(ip in ipmap)) {
      if (nmap >= mapmax) return "[IP]";
      nip++; ipmap[ip] = "ip-" nip;
      nmap++;
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
          (cand !~ /[A-Fa-f]/ && index(cand, "::") == 0 && nc < 6) ||
          cand == "::1") {
        out = out substr(rest, 1, RSTART + RLENGTH - 1);
        rest = substr(rest, RSTART + RLENGTH);
        continue;
      }
      if (!(cand in ip6map)) {
        if (nmap >= mapmax) {
          out = out substr(rest, 1, RSTART - 1) "[IP6]";
          rest = substr(rest, RSTART + RLENGTH);
          continue;
        }
        nip6++; ip6map[cand] = "ip6-" nip6;
        nmap++;
        print "ip6\t" cand "\t" ip6map[cand] >> mapfile;
      }
      out = out substr(rest, 1, RSTART - 1) ip6map[cand];
      rest = substr(rest, RSTART + RLENGTH);
    }
    return out rest;
  }
  function pseudo_fqdn(h,   key) {
    key = tolower(h);
    if (!(key in fqmap)) {
      if (nmap >= mapmax) return "[PRIVATE-HOST]";
      nfq++; fqmap[key] = "private-host-" nfq;
      nmap++;
      print "fqdn\t" h "\t" fqmap[key] >> mapfile;
    }
    return fqmap[key];
  }
  function replace_mapped_fqdns(line,   h, out, idx, pre, nxt, lowerline) {
    # replace every known (pre-seeded or discovered) hostname, with word
    # boundaries so "host1" never corrupts "host10"
    for (h in fqmap) {
      if (length(h) < 4) continue;
      out = "";
      while ((idx = index(tolower(line), tolower(h))) > 0) {
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
  function public_fqdn(fq,   h) {
    h = tolower(fq);
    return h == "netdata.cloud" || h ~ /\.netdata\.cloud$/ ||
           h == "netdata.io" || h ~ /\.netdata\.io$/;
  }
  function known_netdata_filename(fq,   h) {
    h = tolower(fq);
    return h == "netdata.conf" || h == "stream.conf" ||
           h == "exporting.conf" || h == "go.d.conf" ||
           h == "netdata.api.key" || h == "nd.sock";
  }
  function replace_fqdns(line,   out, rest, fq, pre, nxt, ext) {
    # Ordinary public-suffix hostnames can identify a customer just as clearly
    # as private TLDs. A dotted filename is intentionally not exempt because it
    # is indistinguishable from customer.key/customer.sh in arbitrary output.
    out = ""; rest = line;
    while (match(rest, /[A-Za-z0-9][A-Za-z0-9-]*(\.[A-Za-z0-9][A-Za-z0-9-]*)+/)) {
      fq = substr(rest, RSTART, RLENGTH);
      pre = (RSTART > 1) ? substr(rest, RSTART - 1, 1) : "";
      nxt = substr(rest, RSTART + RLENGTH, 1);
      ext = fq; sub(/^.*\./, "", ext);
      if (pre ~ /[A-Za-z0-9._-]/ || nxt ~ /[A-Za-z0-9._-]/ ||
          ext !~ /^[A-Za-z][A-Za-z]+$/ || public_fqdn(fq) || known_netdata_filename(fq)) {
        out = out substr(rest, 1, RSTART + RLENGTH - 1);
        rest = substr(rest, RSTART + RLENGTH);
        continue;
      }
      out = out substr(rest, 1, RSTART - 1) pseudo_fqdn(fq);
      rest = substr(rest, RSTART + RLENGTH);
    }
    return out rest;
  }
  function replace_bounded(line, value, replacement,   out, idx, pre, nxt) {
    if (value == "") return line;
    out = "";
    while ((idx = index(tolower(line), tolower(value))) > 0) {
      pre = (idx > 1) ? substr(line, idx - 1, 1) : "";
      nxt = substr(line, idx + length(value), 1);
      if (pre ~ /[A-Za-z0-9._-]/ || nxt ~ /[A-Za-z0-9._-]/) {
        out = out substr(line, 1, idx + length(value) - 1);
        line = substr(line, idx + length(value));
        continue;
      }
      out = out substr(line, 1, idx - 1) replacement;
      line = substr(line, idx + length(value));
    }
    return out line;
  }
  function replace_host(line, h,   p, key) {
    if (h == "") return line;
    key = tolower(h);
    if (!(key in hostmap)) {
      if (nmap >= mapmax) return replace_bounded(line, h, "[HOST]");
      hostmap[key] = "redacted-host";
      nmap++;
      print "host\t" h "\t" hostmap[key] >> mapfile;
    }
    p = hostmap[key];
    return replace_bounded(line, h, p);
  }
  function replace_user(line, u) {
    if (u == "") return line;
    if (!(u in usermap)) {
      if (nmap >= mapmax) return replace_bounded(line, u, "[USER]");
      usermap[u] = "redacted-user";
      nmap++;
      print "user\t" u "\t" usermap[u] >> mapfile;
    }
    return replace_bounded(line, u, usermap[u]);
  }
  function redact_bearers(line,   out, rest, m, diag) {
    diag = tolower(line);
    if (diag ~ /^[ \t#]*bearer[ \t]+token[ \t]+(protection|support|mode)[ \t]*[=:]/)
      return line;
    out = ""; rest = line;
    while (match(rest, /[Bb][Ee][Aa][Rr][Ee][Rr][ \t]+[A-Za-z0-9._~+\/=-]+/)) {
      m = substr(rest, RSTART, RLENGTH);
      out = out substr(rest, 1, RSTART - 1);
      out = out "Bearer " "[REDACTED]";
      rest = substr(rest, RSTART + RLENGTH);
    }
    return out rest;
  }
  function redact_auth_header(line,   lower, colon) {
    lower = tolower(line);
    sub(/^[ \t]+/, "", lower);
    if (lower !~ /^(authorization|authentication)[ \t]*:/) return line;
    colon = index(line, ":");
    return substr(line, 1, colon) " [REDACTED]";
  }
  {
    # NOTE: no {n,m} regex intervals anywhere in this program - older BSD awks
    # treat them as literal braces, which would silently disable redaction.
    # PEM private keys are multi-line: withhold the WHOLE block, fail closed
    # if the END marker never arrives.
    if (json_comp_forever) next;
    if (json_hold_pending) {
      _pending_start = match($0, /[^ \t]/);
      if (!_pending_start) next;
      json_hold_pending = 0;
      json_start_withholding(substr($0, _pending_start));
      next;
    }
    if (json_hold_top_string) {
      _top_end = json_string_end("\"" $0, 1);
      if (_top_end) json_hold_top_string = 0;
      next;
    }
    if (json_comp_depth > 0) {
      json_composite_scan($0, 1);
      next;
    }
    if (yaml_hold) {
      if ($0 ~ /^[ \t]*$/) next;
      yaml_indent = match($0, /[^ \t]/) - 1;
      if (yaml_indent > yaml_hold_indent) next;
      yaml_hold = 0;
    }
    if (inpem) {
      if ($0 ~ /-----END [A-Z ]*PRIVATE KEY/) inpem = 0;
      next;
    }
    if ($0 ~ /-----BEGIN [A-Z ]*PRIVATE KEY/) {
      print "[REDACTED PRIVATE KEY BLOCK]";
      inpem = 1;
      next;
    }
    line = $0;
    # stream.conf parent side: [<API_KEY>] / [<MACHINE_GUID>] section headers ARE secrets
    if (line ~ /^[ \t]*\[[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]-[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]-[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]-[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]-[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]\][ \t]*$/) {
      print "[REDACTED-KEY-SECTION]"; next;
    }
    line = redact_kv(line);
    if (index(line, "\"") && index(line, ":")) line = redact_json(line);
    # URL userinfo (token@, :password@, or user:password@) and Go DSNs.
    while (match(line, /:\/\/[^\/@ \t[]+@/))
      line = substr(line, 1, RSTART - 1) "://[REDACTED]@" substr(line, RSTART + RLENGTH);
    while (match(line, /[A-Za-z0-9_]+:[^@ \t]+@(tcp|unix)\(/)) {
      m = substr(line, RSTART, RLENGTH);
      line = substr(line, 1, RSTART - 1) "[REDACTED]" substr(m, index(m, "@")) substr(line, RSTART + RLENGTH);
    }
    # JWTs (the eyJ prefix is base64 for double-quote-brace)
    while (match(line, /eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+/))
      line = substr(line, 1, RSTART - 1) "[REDACTED-JWT]" substr(line, RSTART + RLENGTH);
    # HTTP auth values anywhere in a line. Redaction is context-based and does
    # not assume an opaque token contains a digit; only an explicit diagnostic
    # key shape is exempted.
    line = redact_bearers(line);
    while (match(line, /[Bb][Aa][Ss][Ii][Cc][ \t]+[A-Za-z0-9+\/=]+/))
      line = substr(line, 1, RSTART - 1) "Basic [REDACTED]" substr(line, RSTART + RLENGTH);
    line = redact_auth_header(line);
    # secrets passed as URL query parameters (access.log request lines etc.)
    out = ""; rest = line;
    while (match(rest, /[?&][A-Za-z0-9_.-]+=/)) {
      m = substr(rest, RSTART, RLENGTH);
      key = m; sub(/^[?&]/, "", key); sub(/=$/, "", key);
      out = out substr(rest, 1, RSTART + RLENGTH - 1);
      rest = substr(rest, RSTART + RLENGTH);
      if (secret_key(key) && !diagnostic_key(key)) {
        out = out "[REDACTED]";
        sub(/^[^&" \t]+/, "", rest);
      }
    }
    line = out rest;
    # Flag/value form without '=' or ':' (for example --password "value").
    out = ""; rest = line;
    while (match(rest, /--?[A-Za-z0-9_.-]+[ \t]+/)) {
      match_start = RSTART; match_len = RLENGTH;
      flag = substr(rest, match_start, match_len);
      key = flag; sub(/^--?/, "", key); gsub(/[ \t]+$/, "", key);
      if (!secret_key(key) || diagnostic_key(key)) {
        out = out substr(rest, 1, match_start);
        rest = substr(rest, match_start + 1);
        continue;
      }
      value_start = match_start + match_len;
      quote = substr(rest, value_start, 1);
      if (quote == "\"") {
        value_end = json_string_end(rest, value_start);
        value_len = value_end ? value_end - value_start + 1 : length(rest) - value_start + 1;
      } else if (quote == sprintf("%c", 39)) {
        value_end = index(substr(rest, value_start + 1), quote);
        value_len = value_end ? value_end + 1 : length(rest) - value_start + 1;
      } else {
        value_tail = substr(rest, value_start);
        if (match(value_tail, /[ \t]/)) value_len = RSTART - 1;
        else value_len = length(value_tail);
      }
      if (value_len <= 0) {
        out = out substr(rest, 1, match_start + match_len - 1);
        rest = substr(rest, match_start + match_len);
        continue;
      }
      out = out substr(rest, 1, match_start - 1) substr(flag, 1, length(flag) - 1) " [REDACTED]";
      rest = substr(rest, value_start + value_len);
    }
    line = out rest;
    # argv/env-style secrets mid-line (ps output, command lines: -token=X,
    # CLAIM_TOKEN="X"). Quoted values may contain spaces and separators.
    out = ""; rest = line;
    while (match(rest, /[A-Za-z0-9_.-]+[ ]?[=:][ ]?/)) {
      match_start = RSTART; match_len = RLENGTH;
      prefix = substr(rest, match_start, match_len);
      key = prefix; sub(/[ ]?[=:].*/, "", key);
      if (!secret_key(key) || diagnostic_key(key)) {
        # Advance one byte, not the whole non-secret assignment. A wrapper such
        # as "Environment: KEY=value" may contain a secret assignment inside it.
        out = out substr(rest, 1, match_start);
        rest = substr(rest, match_start + 1);
        continue;
      }
      value_start = match_start + match_len;
      quote = substr(rest, value_start, 1);
      value_len = 0;
      if (quote == "\"") {
        value_end = json_string_end(rest, value_start);
        value_len = value_end ? value_end - value_start + 1 : length(rest) - value_start + 1;
      } else if (quote == sprintf("%c", 39)) {
        value_end = index(substr(rest, value_start + 1), quote);
        value_len = value_end ? value_end + 1 : length(rest) - value_start + 1;
      } else {
        value_tail = substr(rest, value_start);
        if (match(value_tail, /[& \t[]/)) value_len = RSTART - 1;
        else value_len = length(value_tail);
      }
      if (value_len == 0) {
        out = out substr(rest, 1, match_start + match_len - 1);
        rest = substr(rest, match_start + match_len);
        continue;
      }
      out = out substr(rest, 1, match_start - 1) key "=[REDACTED]";
      rest = substr(rest, value_start + value_len);
    }
    line = out rest;
    # two-word secret keys mid-line ("api key = X" inside a captured command line)
    out = ""; rest = line;
    while (match(rest, /([Aa][Pp][Ii]|[Ll][Ii][Cc][Ee][Nn][Ss][Ee]|[Aa][Uu][Tt][Hh]|[Aa][Cc][Cc][Ee][Ss][Ss])[ ][Kk][Ee][Yy][ ]?[=:][ ]?[^&" \t[]+|[Pp][Rr][Oo][Xx][Yy][ ]([Uu][Ss][Ee][Rr]|[Pp][Aa][Ss][Ss]([Ww][Oo][Rr][Dd])?)[ ]?[=:][ ]?[^&" \t[]+/)) {
      m = substr(rest, RSTART, RLENGTH);
      sub(/[ ]?[=:].*/, "", m);
      lk = normalize_key(m);
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
      line = replace_fqdns(line);
      line = replace_mapped_fqdns(line);
      if (host_fqdn != "") line = replace_host(line, host_fqdn);
      if (host_short != "") line = replace_host(line, host_short);
      if (run_user != "") line = replace_user(line, run_user);
    }
    print line;
  }'
}

sanitize_file() {
  _f="$1"
  [ -f "$_f" ] || return 0
  if sanitize_stream < "$_f" > "$_f.san" 2>>"$ERRORS"; then
    mv "$_f.san" "$_f"
  else
    # fail CLOSED: never ship content the sanitizer could not process
    rm -f "$_f.san"
    echo "[netdata-sos] sanitization failed for this file - content withheld for safety" > "$_f"
  fi
}

# --- manifest ----------------------------------------------------------------
# manifest_add <rel-path> <kind:cmd|file|api> <origin> <title>
json_str() { # normalize JSON control bytes, then escape backslashes and quotes
  _js="$1"
  LC_ALL=C printf '%s' "$_js" | tr '\001-\037' ' ' | sed 's/\\/\\\\/g; s/"/\\"/g'
}
sanitize_metadata() {
  _metadata_input="$1"
  _metadata=$(LC_ALL=C printf '%s' "$_metadata_input" | tr '\001-\037' ' ')
  _metadata_sanitized=$(printf '%s\n' "$_metadata" | sanitize_stream 2>>"$ERRORS") || {
    printf '%s' '[withheld: metadata sanitization failed]'
    return
  }
  printf '%s' "$_metadata_sanitized"
}
manifest_add() {
  _rel="$1"; _kind="$2"; _origin="$3"; _title="$4"
  _bytes=0; [ -f "$WORK/$_rel" ] && _bytes=$(wc -c < "$WORK/$_rel" | tr -d ' ')
  printf '{"path":"%s","kind":"%s","origin":"%s","title":"%s","bytes":%s,"pii_obfuscated":%s}\n' \
    "$(json_str "$_rel")" "$_kind" "$(json_str "$(sanitize_metadata "$_origin")")" \
    "$(json_str "$(sanitize_metadata "$_title")")" "$_bytes" \
    "$([ "$OBFUSCATE" = "1" ] && echo true || echo false)" >> "$MANIFEST_ROWS"
}

# Keep the first byte budget of an already-sanitized stream while draining the
# remainder. The drain provides backpressure, so a noisy collector can never
# grow an unbounded staging file. Metadata is written as: <bytes> <truncated>.
cap_head_stream() {
  _cap="$1"; _meta="$2"
  LC_ALL=C awk -v cap="$_cap" -v meta="$_meta" '
    BEGIN { kept = 0; total = 0; truncated = 0 }
    {
      record = $0 ORS;
      n = length(record);
      total += n;
      if (kept < cap) {
        remaining = cap - kept;
        if (n <= remaining) { printf "%s", record; kept += n }
        else { printf "%s", substr(record, 1, remaining); kept = cap; truncated = 1 }
      } else truncated = 1;
    }
    END { print total, truncated > meta }
  '
}

finalize_head_cap() {
  _capped="$1"; _dest="$2"; _meta="$3"; _cap="$4"; _marker_mode="$5"
  _total=0; _truncated=0
  read -r _total _truncated < "$_meta" || _truncated=1
  if [ "$_truncated" = "1" ] && [ "$_marker_mode" = "marker" ]; then
    _marker=$(printf '\n### TRUNCATED: sanitized output exceeded %s bytes ###\n' "$_cap")
    _keep=$((_cap - ${#_marker}))
    [ "$_keep" -lt 0 ] && _keep=0
    {
      head -c "$_keep" "$_capped"
      printf '%s\n' "$_marker"
    } > "$_dest"
    rm -f "$_capped"
  else
    mv "$_capped" "$_dest"
  fi
  rm -f "$_meta"
}

PIPE_SEQ=0
sanitize_source_capped() { # <source> <dest> <cap> <head|tail>
  _src="$1"; _dest="$2"; _cap="$3"; _cap_mode="$4"
  PIPE_SEQ=$((PIPE_SEQ + 1))
  _pipe="$STAGING/sanitize.$$.${PIPE_SEQ}.fifo"
  _raw_detect="$STAGING/sanitize.$$.${PIPE_SEQ}.detect.fifo"
  _raw_sanitize="$STAGING/sanitize.$$.${PIPE_SEQ}.text.fifo"
  _od_pipe="$STAGING/sanitize.$$.${PIPE_SEQ}.od.fifo"
  _capped="$_dest.capped"
  _meta="$_dest.capmeta"
  _nul_probe="$_dest.nulprobe"
  rm -f "$_pipe" "$_raw_detect" "$_raw_sanitize" "$_od_pipe" "$_capped" "$_meta" "$_nul_probe"
  mkfifo "$_pipe" "$_raw_detect" "$_raw_sanitize" "$_od_pipe" || return 1
  if [ "$_cap_mode" = "tail" ]; then
    tail -c "$_cap" < "$_pipe" > "$_capped" &
  else
    cap_head_stream "$_cap" "$_meta" < "$_pipe" > "$_capped" &
  fi
  _sinkpid=$!
  sanitize_stream < "$_raw_sanitize" > "$_pipe" 2>>"$ERRORS" & _sanpid=$!
  od -An -v -t u1 < "$_raw_detect" > "$_od_pipe" 2>>"$ERRORS" & _odpid=$!
  awk '{ for (i = 1; i <= NF; i++) if ($i == 0) found = 1 } END { if (found) print "nul" }' \
    < "$_od_pipe" > "$_nul_probe" 2>>"$ERRORS" & _probepid=$!
  # Open the live source once. tee fans that one byte stream to the byte-level
  # encoding detector and text sanitizer; nothing is published unless every
  # consumer succeeds. The watchdog bounds the source read and the sink bounds
  # retained disk.
  run_capped_input "$CMD_TIMEOUT" "$_src" tee "$_raw_detect" "$_raw_sanitize" > /dev/null 2>>"$ERRORS"
  _readrc=$?
  wait "$_sanpid"; _sanrc=$?
  wait "$_odpid"; _odrc=$?
  wait "$_probepid"; _proberc=$?
  wait "$_sinkpid"; _sinkrc=$?
  rm -f "$_pipe" "$_raw_detect" "$_raw_sanitize" "$_od_pipe"
  if [ "$_readrc" -ne 0 ] || [ "$_sanrc" -ne 0 ] || [ "$_odrc" -ne 0 ] ||
     [ "$_proberc" -ne 0 ] || [ "$_sinkrc" -ne 0 ] || [ -s "$_nul_probe" ]; then
    rm -f "$_capped" "$_meta" "$_nul_probe"
    echo "[netdata-sos] sanitization failed or timed out - content withheld for safety" > "$_dest"
    return 1
  fi
  rm -f "$_nul_probe"
  if [ "$_cap_mode" = "tail" ]; then
    mv "$_capped" "$_dest"
  else
    finalize_head_cap "$_capped" "$_dest" "$_meta" "$_cap" plain
  fi
  return 0
}

CAPTURE_RC=0
CAPTURE_SAFE=0
capture_command_output() { # <dest> <cap> <header|raw> <cmdline> <cmd...>
  _dest="$1"; _cap="$2"; _capture_mode="$3"; _cmdline="$4"; shift 4
  PIPE_SEQ=$((PIPE_SEQ + 1))
  _rawpipe="$STAGING/command.$$.${PIPE_SEQ}.raw.fifo"
  _guardpipe="$STAGING/command.$$.${PIPE_SEQ}.guard.fifo"
  _sanpipe="$STAGING/command.$$.${PIPE_SEQ}.san.fifo"
  _rawprobe="$STAGING/command.$$.${PIPE_SEQ}.raw.probe"
  _capped="$_dest.capped"
  _meta="$_dest.capmeta"
  rm -f "$_rawpipe" "$_guardpipe" "$_sanpipe" "$_rawprobe" "$_capped" "$_meta"
  mkfifo "$_rawpipe" "$_guardpipe" "$_sanpipe" || return 1

  cap_head_stream "$_cap" "$_meta" < "$_sanpipe" > "$_capped" &
  _sinkpid=$!
  sanitize_stream < "$_guardpipe" > "$_sanpipe" 2>>"$ERRORS" &
  _sanpid=$!
  # Stream through the sanitizer while retaining only a bounded byte-count
  # probe. If the producer crosses the raw guard, discard the entire capture:
  # a byte boundary is not a safe place to publish partially recognized data.
  (
    # Keep one read descriptor open after head reaches the guard so the writer
    # never receives SIGPIPE; drain the rejected remainder without storing it.
    exec 4< "$_rawpipe"
    head -c $((SANITIZE_INPUT_CAP + 1)) <&4 | tee "$_rawprobe" > "$_guardpipe"
    _forward_rc=$?
    cat <&4 >/dev/null
    exec 4<&-
    [ "$_forward_rc" -eq 0 ]
  ) &
  _guardpid=$!

  exec 3> "$_rawpipe"
  if [ "$_capture_mode" = "header" ]; then
    printf '# netdata-sos v%s | command: %s | captured: %s\n' \
      "$VERSION" "$_cmdline" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >&3
  fi
  _t0=$(now_s)
  run_capped "$CMD_TIMEOUT" "$@" >&3 2>&1
  CAPTURE_RC=$?
  if [ "$_capture_mode" = "header" ]; then
    printf '# exit: %s | duration: %ss\n' "$CAPTURE_RC" "$(( $(now_s) - _t0 ))" >&3
  fi
  exec 3>&-

  wait "$_guardpid"; _guardrc=$?
  wait "$_sanpid"; _sanrc=$?
  wait "$_sinkpid"; _sinkrc=$?
  _rawbytes=$(wc -c < "$_rawprobe" 2>/dev/null | tr -d ' ')
  rm -f "$_rawpipe" "$_guardpipe" "$_sanpipe" "$_rawprobe"
  if [ "$CAPTURE_RC" -ne 0 ] || [ "$_guardrc" -ne 0 ] ||
     [ "$_sanrc" -ne 0 ] || [ "$_sinkrc" -ne 0 ] ||
     [ "${_rawbytes:-0}" -gt "$SANITIZE_INPUT_CAP" ]; then
    rm -f "$_capped" "$_meta"
    echo "[netdata-sos] collection, raw input guard, or sanitization failed - content withheld for safety" > "$_dest"
    CAPTURE_SAFE=0
    return 1
  fi
  if [ "$_capture_mode" = "header" ]; then _marker_mode=marker; else _marker_mode=plain; fi
  finalize_head_cap "$_capped" "$_dest" "$_meta" "$_cap" "$_marker_mode"
  CAPTURE_SAFE=1
  return 0
}

PATH_SEQ=0
SAFE_DYNAMIC_PATH=""
map_dynamic_path() { # <source-relative-path> <label>
  _dynamic_original="$1"; _dynamic_label="$2"
  PATH_SEQ=$((PATH_SEQ + 1))
  _dynamic_ext=${_dynamic_original##*.}
  case "$_dynamic_ext" in
    conf|yml|yaml|dyncfg) : ;;
    *) _dynamic_ext=txt ;;
  esac
  SAFE_DYNAMIC_PATH=$(printf '%s-%03d.%s' "$_dynamic_label" "$PATH_SEQ" "$_dynamic_ext")
  _dynamic_map_value=$(LC_ALL=C printf '%s' "$_dynamic_original" | tr '\001-\037' ' ')
  printf 'path\t%s\t%s\n' "$_dynamic_map_value" "$SAFE_DYNAMIC_PATH" >> "$MAP_FILE"
}

# --- collectors ---------------------------------------------------------------
# collect_cmd <rel-path> <title> <cmd...>
collect_cmd() {
  _rel="$1"; _title="$2"; shift 2
  _out="$WORK/$_rel"
  mkdir -p "$(dirname "$_out")"
  if deadline_exceeded; then
    echo "SKIPPED: global deadline reached" > "$_out"
    manifest_add "$_rel" "cmd" "skipped" "$_title (skipped: deadline)"
    return 0
  fi
  # header must stay a single line even for multi-line sh -c scripts
  # shellcheck disable=SC1003  # literal backslash deletion, not a quote escape
  _cmdline=$(printf '%s' "$*" | tr -d '\\' | tr '\n\t' '  ' | tr -s ' ')
  capture_command_output "$_out" "$API_CAP" header "$_cmdline" "$@" || true
  manifest_add "$_rel" "cmd" "$_cmdline" "$_title"
}

# collect_file <rel-path> <title> <source-file> [cap-bytes]
collect_file() {
  _rel="$1"; _title="$2"; _src="$3"; _cap="${4:-$FILE_CAP}"; _origin_override="${5:-}"
  deadline_exceeded && return 0
  [ -f "$_src" ] && [ -r "$_src" ] || return 0
  path_has_symlink "$_src" && return 0
  _out="$WORK/$_rel"; mkdir -p "$(dirname "$_out")"
  _size=$(wc -c < "$_src" 2>/dev/null | tr -d ' ')
  if [ "${_size:-0}" -gt "$_cap" ]; then
    sanitize_source_capped "$_src" "$_out" "$_cap" tail || true
    _origin="${_origin_override:-$_src} (sanitized tail, $_cap of $_size source bytes)"
  else
    sanitize_source_capped "$_src" "$_out" "$_cap" head || true
    _origin="${_origin_override:-$_src}"
  fi
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
  capture_command_output "$_out" "$API_CAP" raw "$_cmdline" "$@" || true
  if [ "$CAPTURE_SAFE" = "1" ] && [ -s "$_out" ]; then
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
    capture_command_output "$_out" "$API_CAP" raw "curl $_upath" \
      curl -sf --max-time "$CMD_TIMEOUT" "$_url" || true
  elif command -v wget >/dev/null 2>&1; then
    capture_command_output "$_out" "$API_CAP" raw "wget $_upath" \
      wget -q -T "$CMD_TIMEOUT" -O - "$_url" || true
  else
    CAPTURE_SAFE=0
  fi
  if [ "$CAPTURE_SAFE" = "1" ] && [ -s "$_out" ]; then
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
dsn2: user:SENTINEL-7@unix(/run/mysql.sock)/db
TELEGRAM_BOT_TOKEN="SENTINEL-8"
TOKEN=false
PASSWORD=/etc/SENTINEL-9
DB_PASS=SENTINEL-DBPASS
MYSQL_PWD=SENTINEL-MYSQLPWD
GH_PAT=SENTINEL-GHPAT
GET /api/v1/data?chart=x&token=SENTINEL-10&after=-60
/usr/sbin/netdata-claim.sh -token=SENTINEL-11 -rooms=abc
cmdline: /usr/sbin/agent -token=SENTINEL-14 --verbose
Environment: NETDATA_CLAIM_TOKEN=SENTINEL-ENV PATH=/usr/bin
netdata --api-key:SENTINEL-HYPHEN --verbose-hyphen
netdata --password="SENTINEL-QUOTED" --token='SENTINEL-SINGLE' --verbose-quoted
netdata --password "SENTINEL-SPACED" --api-key 'SENTINEL-SPACED-KEY' --verbose-spaced
connect user:SENTINEL-15@unix(/run/x)/db ok
redis://:SENTINEL-URI-PASSWORD@cache.customer.example/0
https://SENTINEL-URI-TOKEN@api.customer.example/path
/etc/netdata/claim_token: SENTINEL-16
destination = [2001:db8::77]:19999 unix:/run/nd.sock
cmdline: claim.sh api key = SENTINEL-12 end
[11111111-2222-3333-4444-555555555555]
-----BEGIN RSA PRIVATE KEY-----
U0VOVElORUwtMTMtUEVNLUJPRFk=
-----END RSA PRIVATE KEY-----
bearer token protection = no
netdata management api key file = /var/lib/netdata/netdata.api.key
TCP SYN cookies = auto
destination = parent.bigcorp.example:19999
destination = tcp:protoparent.example.com:19999
destination = 10.7.7.7:19999
url: https://service.tenant-example.com/metrics
{"password":"SENTINEL-JSON-A\\\"SENTINEL-JSON-B","token":123456789}
{"accessToken":"SENTINEL-CAMEL-A","clientSecret":"SENTINEL-CAMEL-B"}
{"databasePassword":"SENTINEL-DB-CAMEL","dbPassword":"SENTINEL-DB-SHORT","githubToken":"SENTINEL-GITHUB","apiSecret":"SENTINEL-API-SECRET"}
{"pass\u0077ord":"SENTINEL-ESCAPED-KEY"}
{"password":
  "SENTINEL-NEXT-LINE"}
after next-line json
{"credentials": {
  "value": "SENTINEL-NESTED-JSON"
}}
after nested json
password: |
  SENTINEL-YAML-BLOCK
after yaml block
password: |2-
  SENTINEL-YAML-INDICATOR
after yaml indicator
password: &credential |
  SENTINEL-YAML-ANCHOR
after yaml anchor
host testhost and testhost10 /testhost/
mixed TESTHOST and TESTHOST.EXAMPLE.COM
customer.key customer.sh
tcp LISTEN 0 4096 later-line
server at 10.1.2.3 and 2606:4700:10::ac42:aad8 and 2001:470:26:307:0:0:0:1
mail ops@example.com mac aa:bb:cc:dd:ee:ff at 2026-07-16T13:38:34Z
VECTORS
  # Split the scheme so static secret scanners do not mistake this synthetic
  # regression vector for a committed live credential.
  {
    printf 'Authorization: Bear%s %s\n' er SENTINEL-ALPHABETIC
    printf 'WORDTOKEN Authorization: Bear%s token\n' er
    printf 'Authorization: BASIC YTpi\nAuthentication: Digest %s\n' SENTINEL-DIGEST
  } >> "$_tf"
  cat >> "$_tf" <<'MALFORMED_JSON'
{"password": {
  "nested": "SENTINEL-MISMATCH-INNER"
]
SENTINEL-MISMATCH-AFTER
MALFORMED_JSON
  _obf_save="$OBFUSCATE"; OBFUSCATE=1
  _host_short_save="$HOST_SHORT"; HOST_SHORT=testhost
  sanitize_file "$_tf"
  HOST_SHORT="$_host_short_save"
  OBFUSCATE="$_obf_save"
  t_absent() {
    _test_pattern="$1"; _test_desc="$2"
    if grep -q "$_test_pattern" "$_tf"; then echo "FAIL (leak): $_test_desc"; _fails=$((_fails + 1)); fi
  }
  t_present() {
    _test_literal="$1"; shift
    if [ "$_test_literal" = "--" ]; then _test_literal="$1"; shift; fi
    _test_desc="$1"
    if ! grep -qF -- "$_test_literal" "$_tf"; then echo "FAIL (over-redaction): $_test_desc"; _fails=$((_fails + 1)); fi
  }
  t_absent  "SENTINEL-"                  "a planted secret survived"
  t_absent  "123456789"                  "numeric JSON secret survived"
  t_absent  "U0VOVElORUw"                "PEM body survived"
  t_absent  "TOKEN=false"                "TOKEN=false survived (values are never exempt)"
  t_absent  "2606:4700"                  "compressed IPv6 survived"
  t_absent  "2001:470:26:307:0:0:0:1"    "uncompressed numeric IPv6 survived"
  t_absent  "10\.1\.2\.3"             "IPv4 survived"
  t_absent  "parent.bigcorp.example"     "stream destination hostname survived"
  t_absent  "protoparent.example.com"     "protocol-prefixed destination hostname survived"
  t_absent  "service.tenant-example.com"  "ordinary customer FQDN survived"
  t_absent  "customer.key"                "filename-like customer FQDN survived"
  t_absent  "customer.sh"                 "script-like customer FQDN survived"
  t_absent  "TESTHOST"                    "case-varied hostname survived"
  t_absent  "10\.7\.7\.7"              "IP destination not pseudonymized as an IP"
  t_present "tcp LISTEN 0 4096 later-line" "literal tcp corrupted by fqmap pollution"
  t_present "destination = tcp:"          "destination protocol prefix lost"
  t_absent  "ops@example.com"            "email survived"
  t_absent  "aa:bb:cc:dd:ee:ff"          "MAC survived"
  t_present "bearer token protection = no"  "diagnostic option lost (key-based exemption broken)"
  _bearer_expected=$(printf 'WORDTOKEN Authorization: Bear%s [REDACTED]' er)
  t_present "$_bearer_expected" "English-word bearer credential survived"
  t_present "api key file = /var/lib/netdata/netdata.api.key" "key-file path lost"
  t_present "TCP SYN cookies = auto"     "SYN cookies value lost"
  t_present "[REDACTED PRIVATE KEY BLOCK]" "PEM block marker missing"
  t_present '"password":"[REDACTED]","token":"[REDACTED]"' "JSON redaction broke structure"
  t_present "after nested json"          "multiline JSON withholding never resumed"
  t_present "after yaml block"           "YAML block withholding never resumed"
  t_present "after next-line json"       "next-line JSON withholding never resumed"
  t_present "after yaml indicator"       "YAML indicator block withholding never resumed"
  t_present "after yaml anchor"          "YAML anchor block withholding never resumed"
  t_present "2026-07-16T13:38:34Z"       "timestamp mangled by IPv6 rule"
  t_present -- "--verbose"               "path-bearing argv line was eaten by the kv rule"
  t_present -- "--verbose-hyphen"        "hyphenated argv line was eaten by the kv rule"
  t_present -- "--verbose-quoted"        "quoted argv line tail was eaten"
  t_present -- "--verbose-spaced"        "space-separated argv line tail was eaten"
  t_present "@unix(/run/x)/db ok"        "mid-line unix( DSN rule broke the tail"
  t_present "unix:/run/nd.sock"          "socket-path destination was mangled"
  t_absent  "2001:db8::77"               "bracketed IPv6 destination leaked"
  t_present "testhost10"                  "short hostname corrupted a longer word"

  _same_line_file="$STAGING/json-mismatch-same-line.txt"
  printf '%s\n%s\n' \
    '{"password": {"x": [}, "other": "SENTINEL-SAME-LINE"}' \
    'SENTINEL-AFTER-SAME-LINE' > "$_same_line_file"
  sanitize_file "$_same_line_file"
  if grep -q 'SENTINEL' "$_same_line_file"; then
    echo "FAIL: mismatched same-line JSON composite leaked a secret"
    _fails=$((_fails + 1))
  fi

  # High-cardinality input must stop extending the correlation map.
  _map_limit_save=$PSEUDONYM_MAP_MAX; PSEUDONYM_MAP_MAX=1
  _map_size_before=$(wc -c < "$MAP_FILE" | tr -d ' ')
  _map_limit_file="$STAGING/map-limit.txt"
  printf '8.8.4.4 overflow.customer.example\n' > "$_map_limit_file"
  sanitize_file "$_map_limit_file"
  _map_size_after=$(wc -c < "$MAP_FILE" | tr -d ' ')
  PSEUDONYM_MAP_MAX=$_map_limit_save
  if ! grep -q '\[IP\].*\[PRIVATE-HOST\]' "$_map_limit_file" ||
     [ "$_map_size_before" -ne "$_map_size_after" ]; then
    echo "FAIL: pseudonym map cardinality bound failed"
    _fails=$((_fails + 1))
  fi

  # Integration regression: the complete source is sanitized before the tail
  # cap is applied, so slicing can never expose an unrecognizable credential.
  _boundary_src="$STAGING/boundary-source.txt"
  _boundary_out="$STAGING/boundary-output.txt"
  _boundary_line='header=eyJABCDEFGHIJK.SENTINEL-BOUNDARY-PAYLOAD.SENTINEL-BOUNDARY-SIGNATURE'
  printf '%s\n' "$_boundary_line" > "$_boundary_src"
  _fill=$((4096 + 10 - ${#_boundary_line} - 1))
  head -c "$_fill" /dev/zero | tr '\000' X >> "$_boundary_src"
  sanitize_source_capped "$_boundary_src" "$_boundary_out" 4096 tail || _fails=$((_fails + 1))
  if grep -q 'SENTINEL-BOUNDARY' "$_boundary_out" || [ ! -s "$_boundary_out" ]; then
    echo "FAIL: truncation boundary leaked a secret or produced empty output"
    _fails=$((_fails + 1))
  fi

  # File collection must preserve known benign content while removing secrets.
  _source_out="$STAGING/source-output.txt"
  printf 'KEEP-NONSECRET-CONTENT\npassword=SENTINEL-SOURCE\n' > "$_boundary_src"
  sanitize_source_capped "$_boundary_src" "$_source_out" 4096 head || _fails=$((_fails + 1))
  if ! grep -qF 'KEEP-NONSECRET-CONTENT' "$_source_out" || grep -q 'SENTINEL-SOURCE' "$_source_out"; then
    echo "FAIL: source sanitizer dropped benign content or leaked its secret"
    _fails=$((_fails + 1))
  fi

  _nul_source="$STAGING/nul-source.txt"
  _nul_output="$STAGING/nul-output.txt"
  printf 'password=' > "$_nul_source"
  printf '\000SENTINEL-NUL\n' >> "$_nul_source"
  sanitize_source_capped "$_nul_source" "$_nul_output" 4096 head || true
  if grep -q 'SENTINEL-NUL' "$_nul_output" || ! grep -q 'withheld for safety' "$_nul_output"; then
    echo "FAIL: embedded-NUL source was not withheld fail-closed"
    _fails=$((_fails + 1))
  fi

  _newline_root="$STAGING/newline-path-root"
  _newline_dir="$_newline_root/unsafe
"
  mkdir -p "$_newline_dir"
  : > "$_newline_dir/absolute-looking.conf"
  if find "$_newline_root" -type f -exec sh "$PATH_FILTER" {} + | grep -q .; then
    echo "FAIL: newline-bearing discovery path entered line transport"
    _fails=$((_fails + 1))
  fi

  # A source symlink present before collection must be rejected.
  _link_target="$STAGING/symlink-target.txt"
  _link_source="$STAGING/symlink-source.txt"
  printf 'SENTINEL-SYMLINK\n' > "$_link_target"
  if ln -s "$_link_target" "$_link_source" 2>/dev/null; then
    if ! path_has_symlink "$_link_source"; then
      echo "FAIL: source symlink was accepted"
      _fails=$((_fails + 1))
    fi
  fi

  # Validate the Linux process-table fallback independently of ps availability.
  if [ -d /proc ] && ! proc_process_table | awk -v pid="$$" '$1 == pid { found = 1 } END { exit !found }'; then
    echo "FAIL: /proc process-table fallback omitted the current process"
    _fails=$((_fails + 1))
  fi

  # A noisy command is streamed through the sanitizer into a bounded sink.
  _bounded_out="$STAGING/bounded-command.txt"
  capture_command_output "$_bounded_out" 1024 raw "bounded-output-selftest" \
    awk 'BEGIN { for (i = 0; i < 5000; i++) print "bounded-output-line" }' || _fails=$((_fails + 1))
  _bounded_size=$(wc -c < "$_bounded_out" | tr -d ' ')
  if [ "$_bounded_size" -gt 1024 ]; then
    echo "FAIL: command output cap exceeded ($_bounded_size > 1024)"
    _fails=$((_fails + 1))
  fi

  # A producer that crosses the bounded raw-input guard is withheld entirely;
  # a raw byte boundary can never be treated as a safe sanitization boundary.
  _raw_guard_save=$SANITIZE_INPUT_CAP; SANITIZE_INPUT_CAP=4096
  _guarded_out="$STAGING/raw-guard-command.txt"
  capture_command_output "$_guarded_out" 1024 header "raw-guard-selftest" \
    awk 'BEGIN { for (i = 0; i < 5000; i++) print "SENTINEL-RAW-GUARD" }' || true
  SANITIZE_INPUT_CAP=$_raw_guard_save
  if [ "$CAPTURE_SAFE" -ne 0 ] || grep -q 'SENTINEL-RAW-GUARD' "$_guarded_out"; then
    echo "FAIL: oversized raw command output was not withheld fail-closed"
    _fails=$((_fails + 1))
  fi

  # Timeout/abnormal output can end inside an unrecognizable logical record;
  # withhold the entire capture instead of publishing that partial record.
  _timeout_out="$STAGING/timeout-command.txt"
  _timeout_save=$CMD_TIMEOUT; CMD_TIMEOUT=1
  capture_command_output "$_timeout_out" 1024 raw "timeout-selftest" \
    sh -c 'printf "prefix https://admin:SENTINEL-TIMEOUT-PASSWORD"; sleep 30' || true
  CMD_TIMEOUT=$_timeout_save
  if [ "$CAPTURE_SAFE" -ne 0 ] || grep -q 'SENTINEL-TIMEOUT' "$_timeout_out"; then
    echo "FAIL: timed-out partial command output was published"
    _fails=$((_fails + 1))
  fi

  # The watchdog must terminate descendants, not just the immediate sh -c.
  _pidfile="$STAGING/watchdog-pids.txt"
  run_capped 1 sh -c 'sleep 30 & child=$!; echo "$$ $child" > "$1"; wait "$child"' sh "$_pidfile" >/dev/null 2>&1
  _watchdog_rc=$?
  sleep 1
  _orphaned=0
  if [ -s "$_pidfile" ]; then
    _pid_values=$(tr ' ' '\n' < "$_pidfile")
    # shellcheck disable=SC2086  # split the self-test file into numeric PIDs
    for _pid in $_pid_values; do
      kill -0 "$_pid" 2>/dev/null && _orphaned=1
    done
  fi
  if [ "$_watchdog_rc" -ne 124 ] || [ "$_orphaned" -ne 0 ]; then
    echo "FAIL: watchdog rc=$_watchdog_rc orphaned=$_orphaned"
    _fails=$((_fails + 1))
  fi
  # A TERM handler that forks after the first signal must remain contained.
  : > "$_pidfile"
  ND_SOS_DISABLE_SETSID=1; export ND_SOS_DISABLE_SETSID
  run_capped 1 sh -c 'trap '\''sleep 30 & echo $! >> "$1"; sleep 2'\'' TERM; sleep 30 & echo $! >> "$1"; wait' sh "$_pidfile" >/dev/null 2>&1
  _watchdog_rc=$?
  unset ND_SOS_DISABLE_SETSID
  sleep 1
  _orphaned=0
  while IFS= read -r _pid; do
    case "$_pid" in *[!0-9]*|'') continue ;; *) : ;; esac
    kill -0 "$_pid" 2>/dev/null && _orphaned=1
  done < "$_pidfile"
  if [ "$_watchdog_rc" -ne 124 ] || [ "$_orphaned" -ne 0 ]; then
    echo "FAIL: watchdog late-fork rc=$_watchdog_rc orphaned=$_orphaned"
    _fails=$((_fails + 1))
  fi
  if [ "$_fails" -eq 0 ]; then
    echo "netdata-sos selftest: ALL PASS"
    exit 0
  fi
  echo "netdata-sos selftest: $_fails FAILURE(S)" >&2
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
  { curl -sf --max-time 5 "http://127.0.0.1:${NDPORT}/api/v2/node_instances" 2>/dev/null ||
    wget -q -T 5 -O - "http://127.0.0.1:${NDPORT}/api/v2/node_instances" 2>/dev/null; } |
    tr ',{' '\n' | sed -nE 's/.*"(nm|hostname)" *: *"([^"]*)".*/\2/p' |
    awk '!seen[$0]++ { print; if (++count >= 4096) exit }' |
    {
      _seed_n=0
      while IFS= read -r _h; do
        [ "$_h" = "$HOST_SHORT" ] && continue
        [ "$_h" = "$HOST_FQDN" ] && continue
        [ "$_h" = "localhost" ] && continue
        [ ${#_h} -lt 4 ] && continue
        _seed_n=$((_seed_n + 1))
        printf 'fqdn\t%s\tprivate-host-%s\n' "$_h" "$_seed_n" >> "$MAP_FILE"
      done
    }
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
  collect_file 04-config/netdata.conf "On-disk main config" "$CONFDIR/netdata.conf"
  collect_file 04-config/stream.conf "Streaming config (parent/child; api key redacted)" "$CONFDIR/stream.conf"
  collect_file 04-config/exporting.conf "Exporting engine config (credentials redacted)" "$CONFDIR/exporting.conf"
  collect_file 04-config/go.d.conf "go.d orchestrator config (module enable/disable)" "$CONFDIR/go.d.conf"
  # Every user-customized config, including nested directories. Source paths can
  # contain customer identifiers, so bundle entry names are neutral and the
  # original-to-neutral mapping stays only in the private pseudonym map.
  run_capped "$CMD_TIMEOUT" find "$CONFDIR" -type f \( -name '*.conf' -o -name '*.yml' -o -name '*.yaml' \) \
    -exec sh "$PATH_FILTER" {} + 2>/dev/null | head -200 | while IFS= read -r f; do
    case "$f" in */ssl/*|*.pem|*.key) continue ;; *) : ;; esac
    _relc=${f#"$CONFDIR"/}
    case "$_relc" in netdata.conf|stream.conf|exporting.conf|go.d.conf) continue ;; *) : ;; esac
    map_dynamic_path "$_relc" custom-config
    collect_file "04-config/custom/$SAFE_DYNAMIC_PATH" "User config (source path mapped privately; secrets redacted)" "$f" 262144 "user config mapped to $SAFE_DYNAMIC_PATH"
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
    journalctl -u netdata --no-pager -o short-iso --since "-${SINCE_HOURS} hours"
  collect_cmd 05-logs/journal-namespace-netdata.txt "netdata journal namespace (some installs log here)" \
    journalctl --namespace=netdata --no-pager -o short-iso --since "-${SINCE_HOURS} hours"
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
      cat > "$WORK/05-logs/LOGS-ARE-IN-DOCKER.txt" <<EOF
This agent logs to the container's stdout/stderr. Its log history is NOT
available from inside the container and is therefore not included here.

Raw docker logs may contain credentials and identifying data. DO NOT attach
them directly. If support explicitly requests them, capture only the requested
window locally with a private umask:

    umask 077
    docker logs --since ${SINCE_HOURS}h <netdata-container> > netdata-docker.raw.log 2>&1

Review that local file before sharing: remove credential/header/query values
and replace IP addresses, hostnames, email addresses, and customer identifiers.
Save a separate reviewed copy and attach only that copy; keep or delete the raw
file according to your local data-handling policy.
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
for sf in "$LIBDIR/status-netdata.json" "$CACHEDIR/status-netdata.json" /tmp/status-netdata.json /run/status-netdata.json /var/run/status-netdata.json; do
  [ -f "$sf" ] || continue
  if [ -z "$NEWEST_STATUS" ] || [ "$sf" -nt "$NEWEST_STATUS" ]; then NEWEST_STATUS="$sf"; fi
done
[ -n "$NEWEST_STATUS" ] && collect_file 06-state/status-file.json "Daemon status file: LAST EXIT/CRASH RECORD incl. fatal stack trace (read this first for crashes)" "$NEWEST_STATUS"
if [ -n "$LIBDIR" ]; then
  # Paths under the state dir may contain live tokens, hostnames, or job ids.
  # Report only aggregate inventory data; never emit paths or filenames.
  collect_cmd 06-state/state-inventory.txt "State dir aggregate inventory (all paths and filenames withheld)" \
    sh -c '
      root=$1
      printf "directories: "; find "$root" -type d 2>/dev/null | wc -l
      printf "files excluding bearer tokens: "; find "$root" -type f ! -path "*/bearer_tokens/*" 2>/dev/null | wc -l
      printf "bearer token files (names withheld): "; find "$root" -type f -path "*/bearer_tokens/*" 2>/dev/null | wc -l
      printf "total KiB: "; du -sk "$root" 2>/dev/null | awk "{ print \$1 }"
    ' sh "$LIBDIR"
  collect_cmd 06-state/cloud-state.txt "Cloud claim state (claimed_id is safe; token/private.pem are never collected)" sh -c '
    printf "cloud.d file count (names withheld): "; find '"$LIBDIR"'/cloud.d -maxdepth 1 -type f 2>/dev/null | wc -l;
    echo "== claimed_id ==";
    cat '"$LIBDIR"'/cloud.d/claimed_id 2>/dev/null || echo "(no claimed_id file - agent not claimed)"; echo;
    echo "(token and private.pem intentionally NOT collected)"; true'
  collect_file 06-state/health-silencers.json "Persisted alert silencers" "$LIBDIR/health.silencers.json"
  if [ -f "$LIBDIR/god-jobs-statuses.json" ]; then
    collect_file 06-state/go.d-job-statuses.json "go.d collector job states (which jobs run/fail)" "$LIBDIR/god-jobs-statuses.json"
  else
    run_capped "$CMD_TIMEOUT" find "$LIBDIR"/. ! -name . -prune -type f -name '*jobs-statuses*.json' \
      -exec sh "$PATH_FILTER" {} + 2>/dev/null | head -1 |
      while IFS= read -r gjs; do
        deadline_exceeded && break
        collect_file 06-state/go.d-job-statuses.json "go.d collector job states (which jobs run/fail)" "$gjs"
      done
  fi
  if [ -d "$LIBDIR/config" ]; then
    run_capped "$CMD_TIMEOUT" find "$LIBDIR/config"/. ! -name . -prune -type f -name '*.dyncfg' \
      -exec sh "$PATH_FILTER" {} + 2>/dev/null | head -100 |
      while IFS= read -r dc; do
        deadline_exceeded && break
        map_dynamic_path "$(basename "$dc")" dynamic-config
        collect_file "06-state/dyncfg/$SAFE_DYNAMIC_PATH" "Dynamic config created via UI/API (source path mapped privately; secrets redacted)" "$dc" 262144 "dynamic config mapped to $SAFE_DYNAMIC_PATH"
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
  echo "The local Agent API at 127.0.0.1:$NDPORT was unreachable when this bundle was created. The process may still have been running; see summary.txt, 05-logs, and 06-state/status-file.json." > "$WORK/07-runtime/AGENT-API-UNREACHABLE.txt"
  manifest_add 07-runtime/AGENT-API-UNREACHABLE.txt "file" "generated" "Marker: local Agent API unreachable at collection time"
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
- Copied files (configs, logs, json) are sanitized without injected headers;
  their origin is recorded in `MANIFEST.json`.
- `07-runtime/AGENT-API-UNREACHABLE.txt` exists when the local API did not respond;
  the process state is reported separately in `summary.txt`.
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
# build inside the 0700 staging dir, then publish with O_EXCL (set -C) so a
# pre-existing file OR symlink planted in a shared tmp dir can never be
# followed or overwritten (no check/open TOCTOU window)
( cd "$STAGING" && tar czf "$STAGING/bundle.tar.gz" "$BUNDLE" ) || { echo "failed to create tarball" >&2; exit 1; }
if ! ( set -C; cat "$STAGING/bundle.tar.gz" > "$TARBALL" ) 2>/dev/null; then
  echo "refusing to write $TARBALL (a file or symlink already exists there)" >&2
  exit 1
fi

if [ -s "$MAP_FILE" ]; then
  MAP_OUT="$OUTDIR/$BUNDLE.pseudonym-map.tsv"
  if ! ( set -C; cat "$MAP_FILE" > "$MAP_OUT" ) 2>/dev/null; then
    info "WARNING: refusing to overwrite $MAP_OUT; the pseudonym map was DISCARDED. Rerun with --keep-staging if you need it"
    MAP_OUT=""
  fi
fi

TOTAL_S=$(( $(now_s) - START_TS ))
SIZE=$(du -h "$TARBALL" 2>/dev/null | cut -f1)
echo >&2
info "done in ${TOTAL_S}s"
info "bundle:  $TARBALL ($SIZE)"
[ -n "${MAP_OUT:-}" ] && info "pseudonym map (KEEP PRIVATE, do not send): $MAP_OUT"
info "review it:  tar -tzf $TARBALL"
if [ "${DOCKER_LOGS_NEEDED:-0}" = "1" ]; then
  info "IMPORTANT: this agent logs to the container's stdout - its log history is NOT in this bundle."
  info "raw docker logs are unsafe to attach; follow the private review workflow in 05-logs/LOGS-ARE-IN-DOCKER.txt."
fi
info "attach the bundle to your support ticket."
