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
sed -n -e '/^get_default_interface_ip()/,/^}/p' -e '/^emit_runtime_field()/,/^}/p' \
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

# Optional labels clear only when the OS probe succeeded.
label_output=$(/bin/sh -c '
    . "$1"
    HOST_OS_DETECTION=/etc/os-release
    emit_runtime_field NETDATA_HOST_OS_LABEL_CODENAME ""
    HOST_OS_DETECTION=unknown
    emit_runtime_field NETDATA_HOST_OS_LABEL_CODENAME ""
' sh "${functions_script}")
[ "${label_output}" = "$(printf 'A\tNETDATA_HOST_OS_LABEL_CODENAME\t\nF\tNETDATA_HOST_OS_LABEL_CODENAME\t')" ]

# A matching kubelet process must not inject its PID into the detector protocol.
sed -n '/^HOST_IS_K8S_NODE="false"/,/^fi/p' "${system_info_script}" > "${test_dir}/kubernetes.sh"
kubernetes_output=$(/bin/sh -c '
    pgrep() { printf "12345\n"; }
    KUBERNETES_SERVICE_HOST="" KUBERNETES_SERVICE_PORT=""
    . "$1"
    printf "%s\n" "$HOST_IS_K8S_NODE"
' sh "${test_dir}/kubernetes.sh")
[ "${kubernetes_output}" = true ]

# A failed process enumeration must not publish a false Kubernetes result.
kubernetes_output=$(/bin/sh -c '
    pgrep() { return 2; }
    KUBERNETES_SERVICE_HOST="" KUBERNETES_SERVICE_PORT=""
    . "$1"
    printf "%s\n" "$K8S_STATUS"
' sh "${test_dir}/kubernetes.sh")
[ "${kubernetes_output}" = F ]

sed -n '/^sum_disk_sizes()/,/^}/p' "${system_info_script}" >> "${functions_script}"
disk_output=$(/bin/sh -c '. "$1"; printf "12\n34\n" | sum_disk_sizes 1024' sh "${functions_script}")
[ "${disk_output}" = 47104 ]
disk_output=$(/bin/sh -c '. "$1"; printf "" | sum_disk_sizes 1024' sh "${functions_script}")
[ "${disk_output}" = 0 ]
/bin/sh -c '. "$1"; ! printf "12\ninvalid\n" | sum_disk_sizes 1024' sh "${functions_script}"

printf '%s\n' 'system-info shell tests: OK'
