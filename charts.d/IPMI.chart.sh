# 
# (C) 2017 Zane C. Bowers-Hadley <vvelox@vvelox.net>
# GPL v3+
#

#
# Unfotunately floats are not properly handled. That is why we remove
# period for voltage. Every IPMI system I've seen returns voltage with
# two decimal places, meaning removing it effectively multiplies the
# value by 100.
#

IPMI_update_every=5

IPMI_check() {

	require_cmd sudo || return 1
	require_cmd ipmi-sensors || return 1
	
	# if this fails for what ever reason, fail
	# -n supplied as this needs to operate non-interactively
	# requires the following added to sudoers...
	#    netdata ALL = NOPASSWD: /usr/local/sbin/ipmi-sensors
	sudo -n ipmi-sensors > /dev/null || return 1

	return 0;
}

IPMI_create() {

	data=`sudo -n ipmi-sensors`

	echo CHART IPMI.temp \'\' \"Temperature Sensors\" Celsius Temperature Temperature line 1000 $IPMI_update_every
	echo "$data" | awk -F '|' '{ if ($3 ~ "Temp"){ if ($4 !~ "N"){ gsub(" |\t",""); print "DIMENSION " $2}}}'
	echo END

	echo CHART IPMI.volt \'\' \"Voltage Sensors\" \"Volts * 100\" Voltage Voltage line 1000 $IPMI_update_every
	echo "$data" | awk -F '|' '{ if ($3 ~ "Volt"){ if ($4 !~ "N"){ gsub(" |\t",""); print "DIMENSION " $2}}}'
	echo END

	echo CHART IPMI.fan \'\' \"Fan Sensors\" RPM Fans Fans line 1000 $IPMI_update_every
	echo "$data" | awk -F '|' '{ if ($3 ~ "Fan"){ if ($4 !~ "N"){ gsub(" |\t",""); print "DIMENSION " $2}}}'
	echo END
	
	return 0
}

IPMI_update() {

	data=`sudo -n ipmi-sensors`

	echo BEGIN IPMI.temp $1
	echo "$data" | awk -F '|' '{ if ($3 ~ "Temp"){ if ($4 !~ "N"){ gsub(" |\t",""); print "SET " $2 " = " $4}}}'
	echo END

	echo BEGIN IPMI.volt $1
	echo "$data" | awk -F '|' '{ if ($3 ~ "Volt"){ if ($4 !~ "N"){ gsub(" |\t|\\.",""); print "SET " $2 " = " $4}}}'
	echo END

	echo BEGIN IPMI.fan $1
	echo "$data" | awk -F '|' '{ if ($3 ~ "Fan"){ if ($4 !~ "N"){ gsub(" |\t",""); print "SET " $2 " = " $4}}}'
	echo END
	
	return 0
}

