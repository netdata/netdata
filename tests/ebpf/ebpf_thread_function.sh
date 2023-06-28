#!/bin/bash

netdata_ebpf_test_functions() {
    curl -k -o /tmp/ebpf_netdata_test_functions.txt "${1}"
    TEST=$?
    if [ $TEST -ne 0 ]; then
        if [ -f /tmp/ebpf_netdata_test_functions.txt ]; then
            rm /tmp/ebpf_netdata_test_functions.txt
        fi
        echo "Cannot request run a for ${1}."
        exit 1
    fi

    grep "${2}" /tmp/ebpf_netdata_test_functions.txt >/dev/null
    if [ $TEST -ne 0 ]; then
        rm /tmp/ebpf_netdata_test_functions.txt
        echo "Cannot find ${2} in the output."
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

#Test thread requests (mdflush is not enabled, because it is not present in all distributions by default and process is always enabled
#to keep backward compatibility until we create functions for all threads."
for THREAD in "cachestat" "dcstat" "disk" "fd" "filesystem" "hardirq" "mount" "oomkill" "shm" "socket" "softirq" "sync" "swap" "vfs" ;
do
    echo "TESTING ${THREAD}"
    netdata_ebpf_test_functions "${MURL}/api/v1/function?function=ebpf_thread%20enable:${THREAD}:${INTERVAL}%20thread:${THREAD}"
    sleep 17
    netdata_ebpf_test_functions "${MURL}/api/v1/function?function=ebpf_thread%20thread:${THREAD}" "running"
    sleep 17
    netdata_ebpf_test_functions "${MURL}/api/v1/function?function=ebpf_thread%20disable:${THREAD}"
    sleep 6
    netdata_ebpf_test_functions "${MURL}/api/v1/function?function=ebpf_thread%20thread:${THREAD}" "stopped"
done

