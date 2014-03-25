#!/bin/sh

t=$1
x=$(( t + 1 - 1 ))
if [ "$x" -eq 0 ]
	then
	x=1
fi

while [ 1 ]
do
	/sbin/tc qdisc show | cut -d ' ' -f 5 | sort -u | while read x
	do
		echo
		echo "DEVICE $x"
		echo 
		/sbin/tc -s class show dev $x
	done

	sleep $x
done
