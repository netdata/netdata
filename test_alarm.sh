#!/bin/bash
sshpass -p '123' ssh -o StrictHostKeyChecking=no cm@10.10.30.140 '
cat > /tmp/alarm.txt << EOF
alarm: test_cpu_alarm
on: system.cpu
lookup: average -1m over 1s
every: 5s
warn: \$this > 0.1
crit: \$this > 100
info: test
EOF
echo "123" | sudo -S mv /tmp/alarm.txt /etc/netdata/health.d/test_alarm.conf
echo "123" | sudo -S systemctl restart netdata
sleep 5
echo "=== Badge Response ==="
curl -s "http://localhost:19999/api/v1/badge.svg?chart=system.cpu&alarm=test_cpu_alarm"
echo ""
echo "=== Texts ==="
curl -s "http://localhost:19999/api/v1/badge.svg?chart=system.cpu&alarm=test_cpu_alarm" | grep -o "<text[^>]*>[^<]*</text>"
echo "=== Cleanup ==="
echo "123" | sudo -S rm -f /etc/netdata/health.d/test_alarm.conf
echo "123" | sudo -S systemctl restart netdata
'