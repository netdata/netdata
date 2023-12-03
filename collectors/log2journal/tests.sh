#!/usr/bin/env bash

if [ -f "${PWD}/log2journal" ]; then
  log2journal_bin="${PWD}/log2journal"
else
  log2journal_bin="$(which log2journal)"
fi

[ -z "${log2journal_bin}" ] && echo >&2 "Cannot find log2journal binary" && exit 1
echo >&2 "Using: ${log2journal_bin}"

script_dir=$(dirname "$(readlink -f "$0")")
tests="${script_dir}/tests.d"

if [ ! -d "${tests}" ]; then
  echo >&2 "tests directory '${tests}' is not found."
  exit 1
fi

# Create a random directory name in /tmp
tmp=$(mktemp -d /tmp/script_temp.XXXXXXXXXX)

# Function to clean up the temporary directory on exit
cleanup() {
  echo "Cleaning up..."
  rm -rf "$tmp"
}

# Register the cleanup function to run on script exit
trap cleanup EXIT

# Change to the temporary directory
cd "$tmp" || exit 1

# -----------------------------------------------------------------------------

test_log2journal_config() {
  local in="${1}"
  local out="${2}"
  shift 2

  [ -f output ] && rm output

  printf >&2 "running: "
  printf >&2 "%q " "${log2journal_bin}" "${@}"
  printf >&2 "\n"

  "${log2journal_bin}" <"${in}" "${@}" >output 2>&1
  ret=$?

  [ $ret -ne 0 ] && echo >&2 "${log2journal_bin} exited with code: $ret" && cat output && exit 1

  diff --ignore-all-space "${out}" output
  [ $? -ne -0 ] && echo >&2 "${log2journal_bin} output does not match!" && exit 1

  echo >&2 "OK"
  echo >&2

  return 0
}

# test yaml parsing
echo >&2
echo >&2 "Testing full yaml config parsing..."
test_log2journal_config /dev/null "${tests}/full.output" -f "${tests}/full.yaml" --show-config || exit 1

echo >&2 "Testing command line parsing..."
test_log2journal_config /dev/null "${tests}/full.output" --show-config      \
  --prefix=NGINX_                                                           \
  --filename-key NGINX_LOG_FILENAME                                         \
  --inject SYSLOG_IDENTIFIER=nginx-log                                      \
  --inject=SYSLOG_IDENTIFIER2=nginx-log2                                    \
  --inject 'PRIORITY=${NGINX_STATUS}'                                       \
  --inject='NGINX_STATUS_FAMILY=${NGINX_STATUS}${NGINX_METHOD}'             \
  --rewrite 'PRIORITY=//${NGINX_STATUS}/inject,dont-stop'                   \
  --rewrite "PRIORITY=/^[123]/6"                                            \
  --rewrite='PRIORITY=|^4|5'                                                \
  '--rewrite=PRIORITY=-^5-3'                                                \
  --rewrite "PRIORITY=;.*;4"                                                \
  --rewrite 'NGINX_STATUS_FAMILY=|^(?<first_digit>[1-5])|${first_digit}xx'  \
  --rewrite 'NGINX_STATUS_FAMILY=|.*|UNKNOWN'                               \
  --rename TEST1=TEST2                                                      \
  --rename=TEST3=TEST4                                                      \
  --unmatched-key MESSAGE                                                   \
  --inject-unmatched PRIORITY=1                                             \
  --inject-unmatched=PRIORITY2=2                                            \
  --include=".*"                                                            \
  --exclude ".*HELLO.*WORLD.*"                                              \
  '(?x)                                   # Enable PCRE2 extended mode
   ^
   (?<NGINX_REMOTE_ADDR>[^ ]+) \s - \s    # NGINX_REMOTE_ADDR
   (?<NGINX_REMOTE_USER>[^ ]+) \s         # NGINX_REMOTE_USER
   \[
     (?<NGINX_TIME_LOCAL>[^\]]+)          # NGINX_TIME_LOCAL
   \]
   \s+ "
   (?<MESSAGE>
     (?<NGINX_METHOD>[A-Z]+) \s+          # NGINX_METHOD
     (?<NGINX_URL>[^ ]+) \s+
     HTTP/(?<NGINX_HTTP_VERSION>[^"]+)
   )
   " \s+
   (?<NGINX_STATUS>\d+) \s+               # NGINX_STATUS
   (?<NGINX_BODY_BYTES_SENT>\d+) \s+      # NGINX_BODY_BYTES_SENT
   "(?<NGINX_HTTP_REFERER>[^"]*)" \s+     # NGINX_HTTP_REFERER
   "(?<NGINX_HTTP_USER_AGENT>[^"]*)"      # NGINX_HTTP_USER_AGENT' \
    || exit 1

# -----------------------------------------------------------------------------

test_log2journal() {
  local n="${1}"
  local in="${2}"
  local out="${3}"
  shift 3

  printf >&2 "running test No ${n}: "
  printf >&2 "%q " "${log2journal_bin}" "${@}"
  printf >&2 "\n"
  echo >&2 "using as input  : ${in}"
  echo >&2 "expecting output: ${out}"

  [ -f output ] && rm output

  "${log2journal_bin}" <"${in}" "${@}" >output 2>&1
  ret=$?

  [ $ret -ne 0 ] && echo >&2 "${log2journal_bin} exited with code: $ret" && cat output && exit 1

  diff "${out}" output
  [ $? -ne -0 ] && echo >&2 "${log2journal_bin} output does not match! - here is what we got:" && cat output && exit 1

  echo >&2 "OK"
  echo >&2

  return 0
}

echo >&2
echo >&2 "Testing parsing and output..."

test_log2journal 1 "${tests}/json.log" "${tests}/json.output" json
test_log2journal 2 "${tests}/json.log" "${tests}/json-include.output" json --include "OBJECT"
test_log2journal 3 "${tests}/json.log" "${tests}/json-exclude.output" json --exclude "ARRAY[^2]"
test_log2journal 4 "${tests}/nginx-json.log" "${tests}/nginx-json.output" -f "${script_dir}/log2journal.d/nginx-json.yaml"
test_log2journal 5 "${tests}/nginx-combined.log" "${tests}/nginx-combined.output" -f "${script_dir}/log2journal.d/nginx-combined.yaml"
test_log2journal 6 "${tests}/logfmt.log" "${tests}/logfmt.output" -f "${tests}/logfmt.yaml"
test_log2journal 7 "${tests}/logfmt.log" "${tests}/default.output" -f "${script_dir}/log2journal.d/default.yaml"
