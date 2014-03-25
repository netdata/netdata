#!/bin/sh

t=$1
x=$(( t + 1 - 1 ))
if [ "$x" -eq 0 ]
	then
	x=1
fi

echo "MYPID $$"

while [ 1 ]
do
	/sbin/tc qdisc show | cut -d ' ' -f 5 | sort -u | while read x
	do
		echo "BEGIN $x"
		/sbin/tc -s class show dev $x
		if [ -f /var/run/fireqos/ifaces/$x ]
		then
			name=`cat /var/run/fireqos/ifaces/$x`
			# echo "SETDEVICENAME $name"
			interface_classes=
			source /var/run/fireqos/$name.conf
			for x in $interface_classes
			do
					echo "SETCLASSNAME $x"
			done
		fi
		echo "END $x"
	done

	sleep $x
done
