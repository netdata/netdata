#!/bin/bash

netdata_ebpf_test_functions() {
    echo "QUERYING: ${1}"
    curl -k -o /tmp/ebpf_netdata_test_functions.txt "${1}"
    TEST=$?
    if [ $TEST -ne 0 ]; then
        echo "Cannot request run a for ${1}. See '/tmp/ebpf_netdata_test_functions.txt' for more details."
        exit 1
    fi

    grep "${2}" /tmp/ebpf_netdata_test_functions.txt >/dev/null
    TEST=$?
    if [ $TEST -ne 0 ]; then
        echo "Cannot find ${2} in the output. See '/tmp/ebpf_netdata_test_functions.txt' for more details.."
        exit 1
    fi

    rm /tmp/ebpf_netdata_test_functions.txt
}

MURL="http://127.0.0.1:19999"
INTERVAL=60

if [ -n "$1" ]; then
    MURL="$1"
fi

# Check function loaded
netdata_ebpf_test_functions "${MURL}/api/v1/functions" "ebpf_thread"

# Check function help
netdata_ebpf_test_functions "${MURL}/api/v1/function?function=ebpf_thread%20help" "allows user to control eBPF threads"

#Test default request
netdata_ebpf_test_functions "${MURL}/api/v1/function?function=ebpf_thread" "columns"

#Test thread requests . The mdflush is not enabled, because it is not present in all distributions by default.
#Socket is not in the list, because it will have a complete refactory with  next PR
for THREAD in "cachestat" "dc" "disk" "fd" "filesystem" "hardirq" "mount" "oomkill" "process" "shm" "softirq" "sync" "swap" "vfs" ;
do
    echo "TESTING ${THREAD}"
    netdata_ebpf_test_functions "${MURL}/api/v1/function?function=ebpf_thread%20enable:${THREAD}:${INTERVAL}%20thread:${THREAD}"
    sleep 17
    netdata_ebpf_test_functions "${MURL}/api/v1/function?function=ebpf_thread%20thread:${THREAD}" "running"
    sleep 17
    netdata_ebpf_test_functions "${MURL}/api/v1/function?function=ebpf_thread%20disable:${THREAD}"
    sleep 6
    netdata_ebpf_test_functions "${MURL}/api/v1/function?function=ebpf_thread%20thread:${THREAD}" "stopped"
    sleep 6
done

