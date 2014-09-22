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

CHART pi.gpu_temp '' "Pi GPU Temperature" "Celcius degrees" pi '' line 30001 $pi_update_every
DIMENSION gpu_temp gpu absolute 1 1000

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

	# 1. wget the counters page from squid
	# 2. sed to remove spaces; replace . with _; remove spaces around =; prepend each line with: local squid_
	# 3. egrep lines starting with:
	#    local squid_client_http_ then one or more of these a-z 0-9 _ then = and one of more of 0-9
	#    local squid_server_all_ then one or more of these a-z 0-9 _ then = and one of more of 0-9
	# 4. then execute this as a script with the eval
	#
	# be very carefull with eval:
	# prepare the script and always grep at the end the lines that are usefull, so that
	# even if something goes wrong, no other code can be executed

	local cpuTemp=$(cat /sys/class/thermal/thermal_zone0/temp)

	local gpuTemp=$(/opt/vc/bin/vcgencmd measure_temp)
	local gpuTemp=${gpuTemp//\'C/0}
	local gpuTemp=${gpuTemp//temp=/}
	local gpuTemp=${gpuTemp//\./0}

	local cpuSpeed=$(cat /sys/devices/system/cpu/cpu0/cpufreq/cpuinfo_cur_freq)

	# write the result of the work.
	cat <<VALUESEOF
BEGIN pi.cpu_temp $1
SET cpu_temp = $cpuTemp
END

BEGIN pi.gpu_temp $1
SET gpu_temp = $gpuTemp
END

BEGIN pi.clock $1
SET cpu_clock = $cpuSpeed
END

VALUESEOF

	return 0
}

