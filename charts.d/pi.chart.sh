#!/bin/sh

pi_update_every=1

pi_check() {
	return 0
}

pi_create() {
	# create the charts
	cat <<EOF
CHART pi.cpu_temp '' "Pi CPU Temperature" "Celcius degrees" pi '' line 30001 $pi_update_every
DIMENSION cpu_temp cpu absolute 1 1000
CHART pi.clock '' "Pi CPU Clock" "MHz" pi '' line 30003 $pi_update_every
DIMENSION cpu_clock clock absolute 1 1000
EOF
	return 0
}


pi_update() {
	# the first argument to this function is the microseconds since last update
	# pass this parameter to the BEGIN statement (see bellow).

	# do all the work to collect / calculate the values
	# for each dimension
	# remember: KEEP IT SIMPLE AND SHORT

	local cpuTemp=$(cat /sys/class/thermal/thermal_zone0/temp)
	local cpuSpeed=$(cat /sys/devices/system/cpu/cpu0/cpufreq/cpuinfo_cur_freq)

	# write the result of the work.
	cat <<VALUESEOF
BEGIN pi.cpu_temp $1
SET cpu_temp = $cpuTemp
END
BEGIN pi.clock $1
SET cpu_clock = $cpuSpeed
END

VALUESEOF

	return 0
}
