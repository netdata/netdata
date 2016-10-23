#!/usr/bin/env bash
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
$($DIR/alarm-notify.sh 'sysadmin' 'DummyAlarm' '1475604061' '1475604028' '3' '1475604369' '10min_cpu_usage' 'system.cpu' 'cpu' 'WARNING' 'CLEAR' '83' '80' '2@/etc/netdata/health.d/cpu.conf' '360' '0' '%' 'average cpu utilization for the last 45 minutes'})
