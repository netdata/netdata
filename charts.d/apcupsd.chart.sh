# no need for shebang - this file is loaded from charts.d.plugin

# netdata
# real-time performance and health monitoring, done right!
# (C) 2016 Costa Tsaousis <costa@tsaousis.gr>
# GPL v3+
#

apcupsd_ip=127.0.0.1
apcupsd_port=3551

# how frequently to collect UPS data
apcupsd_update_every=10

apcupsd_timeout=3

# the priority of apcupsd related to other charts
apcupsd_priority=90000

apcupsd_get() {
	run -t $apcupsd_timeout apcaccess status "$1:$2"
}

apcupsd_check() {

	# this should return:
	#  - 0 to enable the chart
	#  - 1 to disable the chart

	require_cmd apcaccess || return 1

	run apcupsd_get $apcupsd_ip $apcupsd_port >/dev/null
	if [ $? -ne 0 ]
		then
		error "cannot get information for apcupsd server."
		return 1
	elif [ $(apcupsd_get $apcupsd_ip $apcupsd_port | awk '/^STATUS.*/{ print $3 }') != "ONLINE" ]
		then
		error "APC UPS not online."
		return 1
	fi

	return 0
}

apcupsd_create() {
	# create the charts
	cat <<EOF
CHART apcupsd.charge '' "UPS Charge" "percentage" ups apcupsd.charge area $((apcupsd_priority + 1)) $apcupsd_update_every
DIMENSION battery_charge charge absolute 1 100

CHART apcupsd.battery_voltage '' "UPS Battery Voltage" "Volts" ups apcupsd.battery.voltage line $((apcupsd_priority + 3)) $apcupsd_update_every
DIMENSION battery_voltage voltage absolute 1 100
DIMENSION battery_voltage_nominal nominal absolute 1 100

CHART apcupsd.input_voltage '' "UPS Input Voltage" "Volts" input apcupsd.input.voltage line $((apcupsd_priority + 4)) $apcupsd_update_every
DIMENSION input_voltage voltage absolute 1 100
DIMENSION input_voltage_min min absolute 1 100
DIMENSION input_voltage_max max absolute 1 100

CHART apcupsd.input_frequency '' "UPS Input Frequency" "Hz" input apcupsd.input.frequency line $((apcupsd_priority + 5)) $apcupsd_update_every
DIMENSION input_frequency frequency absolute 1 100

CHART apcupsd.output_voltage '' "UPS Output Voltage" "Volts" output apcupsd.output.voltage line $((apcupsd_priority + 6)) $apcupsd_update_every
DIMENSION output_voltage voltage absolute 1 100
DIMENSION output_voltage_nominal nominal absolute 1 100

CHART apcupsd.load '' "UPS Load" "percentage" ups apcupsd.load area $((apcupsd_priority)) $apcupsd_update_every
DIMENSION load load absolute 1 100

CHART apcupsd.temp '' "UPS Temperature" "Celsius" ups apcupsd.temperature line $((apcupsd_priority + 7)) $apcupsd_update_every
DIMENSION temp temp absolute 1 100

CHART apcupsd.time '' "UPS Time Remaining" "Minutes" ups apcupsd.time area $((apcupsd_priority + 2)) $apcupsd_update_every
DIMENSION time time absolute 1 100

EOF
	return 0
}


apcupsd_update() {
	# the first argument to this function is the microseconds since last update
	# pass this parameter to the BEGIN statement (see bellow).

	# do all the work to collect / calculate the values
	# for each dimension
	# remember: KEEP IT SIMPLE AND SHORT

	apcupsd_get $apcupsd_ip $apcupsd_port | awk "

BEGIN {
	battery_charge = 0;
	battery_voltage = 0;
	battery_voltage_nominal = 0;
	input_voltage = 0;
	input_voltage_min = 0;
	input_voltage_max = 0;
	input_frequency = 0;
        output_voltage = 0;
 	output_voltage_nominal = 0;
	load = 0;
	temp = 0;
	time = 0;
}
/^BCHARGE.*/		{ battery_charge = \$3 * 100 };
/^BATTV.*/		{ battery_voltage = \$3 * 100 };
/^NOMBATTV.*/		{ battery_voltage_nominal = \$3 * 100 };
/^LINEV.*/		{ input_voltage = \$3 * 100 };
/^MINLINEV.*/		{ input_voltage_min = \$3 * 100 };
/^MAXLINEV.*/		{ input_voltage_max = \$3 * 100 };
/^LINEFREQ.*/		{ input_frequency = \$3 * 100 };
/^OUTPUTV.*/		{ output_voltage = \$3 * 100 };
/^NOMOUTV.*/		{ output_voltage_nominal = \$3 * 100 };
/^LOADPCT.*/		{ load = \$3 * 100 };
/^ITEMP.*/		{ temp = \$3 * 100 };
/^TIMELEFT.*/		{ time = \$3 * 100 };
END {
	print \"BEGIN apcupsd.charge $1\";
	print \"SET battery_charge = \" battery_charge;
	print \"END\"

	print \"BEGIN apcupsd.battery_voltage $1\";
	print \"SET battery_voltage = \" battery_voltage;
	print \"SET battery_voltage_nominal = \" battery_voltage_nominal;
	print \"END\"

	print \"BEGIN apcupsd.input_voltage $1\";
	print \"SET input_voltage = \" input_voltage;
	print \"SET input_voltage_min = \" input_voltage_min;
	print \"SET input_voltage_max = \" input_voltage_max;
	print \"END\"

	print \"BEGIN apcupsd.input_frequency $1\";
	print \"SET input_frequency = \" input_frequency;
	print \"END\"

	print \"BEGIN apcupsd.output_voltage $1\";
	print \"SET output_voltage = \" output_voltage;
        print \"SET output_voltage_nominal = \" output_voltage_nominal;
	print \"END\"

	print \"BEGIN apcupsd.load $1\";
	print \"SET load = \" load;
	print \"END\"

	print \"BEGIN apcupsd.temp $1\";
	print \"SET temp = \" temp;
	print \"END\"

	print \"BEGIN apcupsd.time $1\";
	print \"SET time = \" time;
	print \"END\"
}"
	[ $? -ne 0 ] && error "failed to get values" && return 1

	return 0
}
