#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later

set -eu

test_dir=$(mktemp -d "${TMPDIR:-/tmp}/netdata-system-info-test.XXXXXX")
trap 'rm -rf "${test_dir}"' EXIT HUP INT TERM

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
system_info_script="${script_dir}/../src/daemon/system-info.sh"
functions_script="${test_dir}/functions.sh"

sed -n \
    -e '/^SYSTEM_INFO_MODE=/p' \
    -e '/^os_release_unescape()/,/^}/p' \
    -e '/^load_os_release()/,/^}/p' \
    -e '/^load_lsb_release()/,/^}/p' \
    -e '/^filter_host_os_label_version()/,/^}/p' \
    -e '/^set_host_os_label_versions()/,/^}/p' \
    "${system_info_script}" > "${functions_script}"

os_release_file="${test_dir}/os-release"
printf '%s\n' \
    'NAME="literal $(printf not-executed)"' \
    'VERSION="a\\bc\\d"' \
    'VERSION_ID="24.04"' \
    'UNSUPPORTED="ignored"' > "${os_release_file}"

os_release_output=$(/bin/sh -c '
    . "$1"
    HOST_NAME=unknown HOST_VERSION=unknown HOST_VERSION_ID=unknown
    load_os_release HOST "$2"
    printf "%s|%s|%s\n" "$HOST_NAME" "$HOST_VERSION" "$HOST_VERSION_ID"
' sh "${functions_script}" "${os_release_file}")

expected_output='literal $(printf not-executed)|a\bc\d|24.04'
[ "${os_release_output}" = "${expected_output}" ] || {
    printf 'unexpected os-release output: %s\n' "${os_release_output}" >&2
    exit 1
}

lsb_release_file="${test_dir}/lsb-release"
printf '%s\n' 'DISTRIB_ID=Test' > "${lsb_release_file}"

invalid_lsb_release_path="${test_dir}/invalid-lsb-release"
mkdir "${invalid_lsb_release_path}"
/bin/sh -c '. "$1"; ! load_lsb_release "$2"' sh "${functions_script}" "${invalid_lsb_release_path}"

if [ "$(id -u)" -ne 0 ]; then
    chmod 000 "${lsb_release_file}"
    /bin/sh -c '. "$1"; ! load_lsb_release "$2"' sh "${functions_script}" "${lsb_release_file}"
fi

label_output=$(/bin/sh -c '
    . "$1"
    KERNEL_NAME=Linux HOST_VERSION_ID=24.04 HOST_VERSION=Ubuntu
    set_host_os_label_versions
    printf "%s|%s\n" "$HOST_OS_LABEL_VERSION" "$HOST_OS_LABEL_RELEASE"
' sh "${functions_script}")
[ "${label_output}" = '|24.04' ] || {
    printf 'unexpected duplicate Linux label output: %s\n' "${label_output}" >&2
    exit 1
}

label_output=$(/bin/sh -c '
    . "$1"
    KERNEL_NAME=Linux HOST_OS_LABEL_VERSION=24.04 HOST_OS_LABEL_RELEASE=24.04.3
    filter_host_os_label_version
    printf "%s|%s\n" "$HOST_OS_LABEL_VERSION" "$HOST_OS_LABEL_RELEASE"
' sh "${functions_script}")
[ "${label_output}" = '24.04|24.04.3' ] || {
    printf 'unexpected distinct Linux label output: %s\n' "${label_output}" >&2
    exit 1
}

label_output=$(/bin/sh -c '
    . "$1"
    KERNEL_NAME=Darwin HOST_VERSION_ID=24.04 HOST_VERSION=macOS
    set_host_os_label_versions
    printf "%s|%s\n" "$HOST_OS_LABEL_VERSION" "$HOST_OS_LABEL_RELEASE"
' sh "${functions_script}")
[ "${label_output}" = '24.04|24.04' ] || {
    printf 'unexpected non-Linux label output: %s\n' "${label_output}" >&2
    exit 1
}

# Exercise the network probe with command outcomes, not fixture state mutation.
sed -n -e '/^get_default_interface_ip()/,/^}/p' -e '/^normalize_runtime_value()/,/^}/p' \
    -e '/^emit_runtime_field()/,/^}/p' -e '/^emit_runtime_network()/,/^}/p' \
    "${system_info_script}" >> "${functions_script}"
mkdir "${test_dir}/bin"
cat > "${test_dir}/bin/timeout" <<'EOF'
#!/bin/sh
if [ -n "${TIMEOUT_TEST_LOG:-}" ]; then printf '%s\n' invoked >> "${TIMEOUT_TEST_LOG}"; fi
shift
exec "$@"
EOF
cat > "${test_dir}/bin/route" <<'EOF'
#!/bin/sh
case "$NETWORK_TEST" in
  present|address-failed) printf 'interface: test0\n' ;;
  *) exit 1 ;;
esac
EOF
cat > "${test_dir}/bin/netstat" <<'EOF'
#!/bin/sh
case "$NETWORK_TEST" in
  absent) printf 'Destination Gateway Flags Netif\n' ;;
  *) exit 1 ;;
esac
EOF
cat > "${test_dir}/bin/ifconfig" <<'EOF'
#!/bin/sh
case "$NETWORK_TEST" in
  present) printf 'inet 192.0.2.4 netmask 0xffffff00\n' ;;
  *) exit 1 ;;
esac
EOF
chmod +x "${test_dir}/bin/"*
for agent_mode in --bounded --runtime; do
    : > "${test_dir}/timeout.log"
    PATH="${test_dir}/bin:${PATH}" NETWORK_TEST=present TIMEOUT_TEST_LOG="${test_dir}/timeout.log" /bin/sh -c '
        functions_file=$1
        set -- "$2"
        . "$functions_file"
        KERNEL_NAME=Darwin
        get_default_interface_ip -4
        [ "$NETWORK_STATUS" = V ]
    ' sh "${functions_script}" "${agent_mode}"
    [ ! -s "${test_dir}/timeout.log" ] || {
        printf 'Agent mode %s must keep probes in the supervised process group\n' "${agent_mode}" >&2
        exit 1
    }
done
: > "${test_dir}/timeout.log"
PATH="${test_dir}/bin:${PATH}" NETWORK_TEST=present TIMEOUT_TEST_LOG="${test_dir}/timeout.log" /bin/sh -c '
    . "$1"
    KERNEL_NAME=Darwin
    get_default_interface_ip -4
    [ "$NETWORK_STATUS" = V ]
' sh "${functions_script}"
[ -s "${test_dir}/timeout.log" ]
for network_case in present absent failed address-failed; do
    network_output=$(PATH="${test_dir}/bin:${PATH}" NETWORK_TEST="${network_case}" /bin/sh -c '
        . "$1"
        KERNEL_NAME=Darwin
        get_default_interface_ip -4
        printf "%s|%s|%s\n" "$NETWORK_STATUS" "$DEFAULT_INTERFACE_NAME" "$DEFAULT_INTERFACE_IP"
    ' sh "${functions_script}")
    case "${network_case}" in
        present) expected='V|test0|192.0.2.4' ;;
        absent) expected='A|unknown|unknown' ;;
        *) expected='F|unknown|unknown' ;;
    esac
    [ "${network_output}" = "${expected}" ] || {
        printf 'network %s: expected %s, got %s\n' "${network_case}" "${expected}" "${network_output}" >&2
        exit 1
    }
done

# Recovery must use new output rather than a prior failed or absent probe.
network_output=$(PATH="${test_dir}/bin:${PATH}" /bin/sh -c '
    . "$1"
    KERNEL_NAME=Darwin
    export NETWORK_TEST=failed
    get_default_interface_ip -4
    [ "$NETWORK_STATUS" = F ] || exit 1
    NETWORK_TEST=present
    get_default_interface_ip -4
    printf "%s|%s|%s\n" "$NETWORK_STATUS" "$DEFAULT_INTERFACE_NAME" "$DEFAULT_INTERFACE_IP"
' sh "${functions_script}")
[ "${network_output}" = 'V|test0|192.0.2.4' ]

# Run the actual script, with startup-only probes recorded by PATH shims.
mkdir "${test_dir}/runtime-bin"
cat > "${test_dir}/runtime-bin/probe" <<'EOF'
#!/bin/sh
probe=${0##*/}
printf '%s %s\n' "$probe" "$*" >> "$RUNTIME_PROBE_LOG"
case "$probe:$*" in
    uname:-s) printf '%s\n' "${TEST_KERNEL:-Darwin}" ;;
    uname:-m) printf 'x86_64\n' ;;
    uname:*) printf 'test-kernel\n' ;;
    sysctl:'-n hw.logicalcpu'|sysctl:'-n kern.smp.cpus')
        [ "${TEST_CPU_FAIL:-false}" = false ] || exit 1
        printf '%b\n' "${TEST_CPU:-8}" ;;
    sysctl:'-n hw.memsize'|sysctl:'-n hw.physmem')
        [ "${TEST_RAM_FAIL:-false}" = false ] || exit 1
        printf '%b\n' "${TEST_RAM:-1073741824}" ;;
    sysctl:*) printf '%s\n' "${TEST_FREQUENCY:-2000000000}" ;;
    diskutil:*) printf 'Disk Size: 1 GB (1073741824 Bytes)\n' ;;
    lsvfs:*) printf 'ufs\n' ;;
    df:*)
        [ "${TEST_DISK_FAIL:-false}" = false ] || exit 1
        printf 'total %s\n' "${TEST_DISK_KB:-1048576}" ;;
    route:*)
        case "${NETWORK_TEST:-present}" in
            present|address-failed|no-address) printf 'interface: test0\n' ;;
            unsafe) printf 'interface: test0\r\n' ;;
            *) exit 1 ;;
        esac ;;
    netstat:*) [ "${NETWORK_TEST:-present}" = absent ] || exit 1 ;;
    ifconfig:*)
        [ "${NETWORK_TEST:-present}" != address-failed ] || exit 1
        [ "${NETWORK_TEST:-present}" != no-address ] || exit 0
        printf 'inet 192.0.2.4 netmask 0xffffff00\n' ;;
    lscpu:*) printf 'CPU(s): 8\nModel name: Cortex-A55\nModel name: Cortex-A76\nCPU MHz: %s\n' "${TEST_FREQUENCY:-1800}" ;;
    nproc:*)
        [ "$*" = --all ] || exit 1
        [ "${TEST_CPU_FAIL:-false}" = false ] || exit 1
        printf '%b\n' "${TEST_CPU:-8}" ;;
    ip:*)
        case "${NETWORK_TEST:-present}:$*" in
            absent:*) exit 0 ;;
            failed:*|address-failed:*) exit 1 ;;
            *:'-o -4 route list default') printf 'default via 192.0.2.1 dev test0\n' ;;
            *:'-o -4 addr show dev test0') printf '1: test0 inet 192.0.2.4/24 scope global test0\n' ;;
            *) exit 1 ;;
        esac ;;
    grep:*)
        case "$*" in
            '-q ^lxcfs /proc /proc/self/mounts')
                [ "${TEST_LXCFS:-false}" = true ] ;;
            '-c ^processor /proc/cpuinfo') printf '3\n' ;;
            '-F MemTotal /proc/meminfo')
                [ "${TEST_RAM_FAIL:-false}" = false ] || exit 1
                printf 'MemTotal: %s kB\n' "${TEST_RAM_KB:-1048576}" ;;
            *) exec "$SYSTEM_INFO_TEST_GREP" "$@" ;;
        esac ;;
    awk:*)
        case "$*" in
            *'/proc/net/route')
                case "${NETWORK_TEST:-present}" in
                    present|address-failed) printf 'test0\n' ;;
                    absent) exit 0 ;;
                    *) exit 1 ;;
                esac ;;
            *) exec "$SYSTEM_INFO_TEST_AWK" "$@" ;;
        esac ;;
    systemd-detect-virt:*) printf 'none\n' ;;
    pgrep:*) exit 1 ;;
    curl:*) exit 28 ;;
    *) exit 1 ;;
esac
EOF
for probe in uname sysctl diskutil lsvfs df route netstat ifconfig lscpu nproc ip grep awk systemd-detect-virt pgrep curl dmidecode sw_vers; do
    ln -s probe "${test_dir}/runtime-bin/${probe}"
done
chmod +x "${test_dir}/runtime-bin/probe"
SYSTEM_INFO_TEST_GREP=$(command -v grep)
SYSTEM_INFO_TEST_AWK=$(command -v awk)
export SYSTEM_INFO_TEST_GREP SYSTEM_INFO_TEST_AWK
: > "${test_dir}/runtime-probes.log"
runtime_output=$(PATH="${test_dir}/runtime-bin:${PATH}" RUNTIME_PROBE_LOG="${test_dir}/runtime-probes.log" \
    /bin/sh "${system_info_script}" --runtime)
assert_runtime_frame() {
    printf '%s\n' "$1" | awk -F '\t' '
        NR == 1 { if ($0 != "NETDATA_SYSTEM_INFO_V1") exit 1; next }
        NR == 8 { if ($0 != "NETDATA_SYSTEM_INFO_END") exit 1; next }
        NR > 8 { exit 1 }
        {
            if (NF != 3 || $1 !~ /^[VAFU]$/ || seen[$2]++) exit 1
            if ($2 !~ /^NETDATA_SYSTEM_(CPU_LOGICAL_CPU_COUNT|TOTAL_RAM|TOTAL_DISK_SIZE|DEFAULT_INTERFACE_(NAME|IP|DETECTION))$/) exit 1
            if ($1 != "V" && $3 != "") exit 1
        }
        END { if (NR != 8) exit 1 }
    ' || { printf 'invalid runtime frame: %s\n' "$1" >&2; exit 1; }
}
assert_record() {
    printf '%s\n' "$runtime_output" | grep -Fx "$(printf '%s\t%s\t%s' "$1" "$2" "$3")" >/dev/null || {
        printf 'missing record %s %s %s in: %s\n' "$1" "$2" "$3" "$runtime_output" >&2
        exit 1
    }
}
run_runtime() {
    : > "${test_dir}/runtime-probes.log"
    runtime_output=$(env PATH="${test_dir}/runtime-bin:${PATH}" RUNTIME_PROBE_LOG="${test_dir}/runtime-probes.log" \
        "$@" /bin/sh "${system_info_script}" --runtime)
    assert_runtime_frame "$runtime_output"
    if grep -E '^(lscpu|dmidecode|systemd-detect-virt|pgrep|curl|sw_vers) |^uname -(r|m|p|i)|^sysctl .*freq' "${test_dir}/runtime-probes.log"; then
        printf 'runtime invoked a startup-only probe\n' >&2
        exit 1
    fi
}
assert_runtime_frame "$runtime_output"
run_runtime
assert_record V NETDATA_SYSTEM_CPU_LOGICAL_CPU_COUNT 8
assert_record V NETDATA_SYSTEM_TOTAL_RAM 1073741824
assert_record V NETDATA_SYSTEM_TOTAL_DISK_SIZE 1073741824
assert_record V NETDATA_SYSTEM_DEFAULT_INTERFACE_NAME test0
assert_record V NETDATA_SYSTEM_DEFAULT_INTERFACE_IP 192.0.2.4
assert_record V NETDATA_SYSTEM_DEFAULT_INTERFACE_DETECTION route

# A changing frequency and heterogeneous CPU inventory must not enter runtime.
initial_runtime_output=$runtime_output
run_runtime TEST_FREQUENCY=2400
[ "$runtime_output" = "$initial_runtime_output" ]
run_runtime TEST_CPU=16 TEST_RAM=2147483648
assert_record V NETDATA_SYSTEM_CPU_LOGICAL_CPU_COUNT 16
assert_record V NETDATA_SYSTEM_TOTAL_RAM 2147483648
run_runtime TEST_CPU_FAIL=true TEST_RAM_FAIL=true
assert_record F NETDATA_SYSTEM_CPU_LOGICAL_CPU_COUNT ''
assert_record F NETDATA_SYSTEM_TOTAL_RAM ''
assert_record V NETDATA_SYSTEM_DEFAULT_INTERFACE_IP 192.0.2.4

# Scalar normalization is per-field: later lines are ignored, CR/TAB fail.
run_runtime TEST_CPU='12\nignored' TEST_RAM='2147483648\nignored'
assert_record V NETDATA_SYSTEM_CPU_LOGICAL_CPU_COUNT 12
assert_record V NETDATA_SYSTEM_TOTAL_RAM 2147483648
for unsafe in '12\r' '12\tbad'; do
    run_runtime TEST_CPU="$unsafe"
    assert_record F NETDATA_SYSTEM_CPU_LOGICAL_CPU_COUNT ''
    assert_record V NETDATA_SYSTEM_TOTAL_RAM 1073741824
done
for network_case in absent failed address-failed unsafe no-address; do
    run_runtime NETWORK_TEST="$network_case"
    case "$network_case" in absent|no-address) network_status=A ;; *) network_status=F ;; esac
    for field in NAME IP DETECTION; do
        assert_record "$network_status" "NETDATA_SYSTEM_DEFAULT_INTERFACE_$field" ''
    done
done

# FreeBSD uses only the count sysctl, with disk failure independent of RAM.
run_runtime TEST_KERNEL=FreeBSD TEST_FREQUENCY=2400
assert_record V NETDATA_SYSTEM_CPU_LOGICAL_CPU_COUNT 8
assert_record V NETDATA_SYSTEM_TOTAL_RAM 1073741824
assert_record V NETDATA_SYSTEM_TOTAL_DISK_SIZE 1073741824
run_runtime TEST_KERNEL=FreeBSD TEST_DISK_KB=0
assert_record V NETDATA_SYSTEM_TOTAL_DISK_SIZE 0
run_runtime TEST_KERNEL=FreeBSD TEST_DISK_FAIL=true
assert_record F NETDATA_SYSTEM_TOTAL_DISK_SIZE ''
assert_record V NETDATA_SYSTEM_TOTAL_RAM 1073741824

# Check Linux count behavior on either host; procfs cases need a Linux host.
run_runtime TEST_KERNEL=Linux
assert_record V NETDATA_SYSTEM_CPU_LOGICAL_CPU_COUNT 8
grep -Fx 'nproc --all' "${test_dir}/runtime-probes.log" >/dev/null
if [ -r /proc/meminfo ] && [ -r /proc/net/route ]; then
    run_runtime TEST_KERNEL=Linux TEST_CPU=16 TEST_RAM_KB=2097152
    assert_record V NETDATA_SYSTEM_CPU_LOGICAL_CPU_COUNT 16
    assert_record V NETDATA_SYSTEM_TOTAL_RAM 2147483648
    assert_record V NETDATA_SYSTEM_DEFAULT_INTERFACE_IP 192.0.2.4
    run_runtime TEST_KERNEL=Linux TEST_LXCFS=true
    assert_record V NETDATA_SYSTEM_CPU_LOGICAL_CPU_COUNT 3
    run_runtime TEST_KERNEL=Linux TEST_CPU_FAIL=true TEST_RAM_FAIL=true
    assert_record F NETDATA_SYSTEM_CPU_LOGICAL_CPU_COUNT ''
    assert_record F NETDATA_SYSTEM_TOTAL_RAM ''
    run_runtime NETWORK_TEST=absent TEST_KERNEL=Linux
    assert_record A NETDATA_SYSTEM_DEFAULT_INTERFACE_NAME ''
fi

# Startup still probes inventory and exports legacy missing-network values.
: > "${test_dir}/runtime-probes.log"
startup_output=$(PATH="${test_dir}/runtime-bin:${PATH}" RUNTIME_PROBE_LOG="${test_dir}/runtime-probes.log" \
    NETWORK_TEST=absent /bin/sh "${system_info_script}" --bounded)
printf '%s\n' "$startup_output" | grep -Fx 'NETDATA_SYSTEM_CPU_MODEL=Cortex-A55' >/dev/null
grep -E '^lscpu ' "${test_dir}/runtime-probes.log" >/dev/null
grep -E '^systemd-detect-virt ' "${test_dir}/runtime-probes.log" >/dev/null
grep -E '^pgrep ' "${test_dir}/runtime-probes.log" >/dev/null
for record in NAME=unknown IP=unknown DETECTION=none; do
    printf '%s\n' "$startup_output" | grep -Fx "NETDATA_SYSTEM_DEFAULT_INTERFACE_${record}" >/dev/null
done
plain_output=$(PATH="${test_dir}/runtime-bin:${PATH}" RUNTIME_PROBE_LOG="${test_dir}/runtime-probes.log" \
    NETWORK_TEST=absent /bin/sh "${system_info_script}")
[ "$plain_output" = "$startup_output" ]
startup_output=$(PATH="${test_dir}/runtime-bin:${PATH}" RUNTIME_PROBE_LOG="${test_dir}/runtime-probes.log" \
    NETWORK_TEST=no-address /bin/sh "${system_info_script}" --bounded)
for record in NAME=unknown IP=unknown DETECTION=route; do
    printf '%s\n' "$startup_output" | grep -Fx "NETDATA_SYSTEM_DEFAULT_INTERFACE_${record}" >/dev/null
done

# A matching kubelet process must not inject its PID into the detector protocol.
sed -n '/^HOST_IS_K8S_NODE="false"/,/^fi/p' "${system_info_script}" > "${test_dir}/kubernetes.sh"
kubernetes_output=$(/bin/sh -c '
    pgrep() { printf "12345\n"; }
    KUBERNETES_SERVICE_HOST="" KUBERNETES_SERVICE_PORT=""
    . "$1"
    printf "%s\n" "$HOST_IS_K8S_NODE"
' sh "${test_dir}/kubernetes.sh")
[ "${kubernetes_output}" = true ]

sed -n '/^sum_disk_sizes()/,/^}/p' "${system_info_script}" >> "${functions_script}"
disk_output=$(/bin/sh -c '. "$1"; printf "12\n34\n" | sum_disk_sizes 1024' sh "${functions_script}")
[ "${disk_output}" = 47104 ]
disk_output=$(/bin/sh -c '. "$1"; printf "" | sum_disk_sizes 1024' sh "${functions_script}")
[ "${disk_output}" = 0 ]
/bin/sh -c '. "$1"; ! printf "12\ninvalid\n" | sum_disk_sizes 1024' sh "${functions_script}"

printf '%s\n' 'system-info shell tests: OK'
