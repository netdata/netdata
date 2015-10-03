#!/bin/sh

# if this chart is called X.chart.sh, then all functions and global variables
# must start with X_

# _update_every is a special variable - it holds the number of seconds
# between the calls of the _update() function
sensors_update_every=

sensors_find_all_files() {
	find $1 -name temp\?_input -o -name temp -o -name in\?_input -o -name fan\?_input 2>/dev/null
}

sensors_find_all_dirs() {
	sensors_find_all_files $1 | while read
	do
		dirname $REPLY
	done | sort -u
}

# _check is called once, to find out if this chart should be enabled or not
sensors_check() {

	# this should return:
	#  - 0 to enable the chart
	#  - 1 to disable the chart

	[ ! -z "$( sensors_find_all_files /sys/devices/ )" ] && return 0
	return 1
}

# _create is called once, to create the charts
sensors_create() {
	local path= dir= name= x= file= lfile= labelname= labelid= device= subsystem= id= type= mode= files= multiplier= divisor=

	echo >$TMP_DIR/temp.sh "sensors_update() {"

	for path in $( sensors_find_all_dirs /sys/devices/ | sort -u )
	do
		dir=$( basename $path )
		device=
		subsystem=
		id=
		type=
		name=

		[ -h $path/device ] && device=$( readlink -f $path/device )
		[ ! -z "$device" ] && device=$( basename $device )
		[ -z "$device" ] && device="$dir"

		[ -h $path/subsystem ] && subsystem=$( readlink -f $path/subsystem )
		[ ! -z "$subsystem" ] && subsystem=$( basename $subsystem )
		[ -z "$subsystem" ] && subsystem="$dir"

		[ -f $path/name ] && name=$( cat $path/name )
		[ -z "$name" ] && name="$dir"

		[ -f $path/type ] && type=$( cat $path/type )
		[ -z "$type" ] && type="$dir"

		id="$( fixid "$device.$subsystem.$dir" )"

		echo >&2 "charts.d: temperature sensors on path='$path', dir='$dir', device='$device', subsystem='$subsystem', id='$id', name='$name'"

		for mode in temperature voltage fans
		do
			files=
			multiplier=1
			divisor=1
			case $mode in
				temperature)
					files="$( find $path -name temp\?_input -o -name temp | sort -u)"
					[ -z "$files" ] && continue
					echo "CHART sensors.temp_${id} '' '${name} Temperature' 'Temperature' 'Celcius Degrees' '' line 6000 $sensors_update_every"
					echo >>$TMP_DIR/temp.sh "echo \"BEGIN sensors.temp_${id} \$1\""
					divisor=1000
					;;

				voltage)
					files="$( find $path -name in\?_input )"
					[ -z "$files" ] && continue
					echo "CHART sensors.volt_${id} '' '${name} Voltage' 'Voltage' 'Volts' '' line 6001 $sensors_update_every"
					echo >>$TMP_DIR/temp.sh "echo \"BEGIN sensors.volt_${id} \$1\""
					divisor=1000
					;;

				fans)
					files="$( find $path -name fan\?_input )"
					[ -z "$files" ] && continue
					echo "CHART sensors.fan_${id} '' '${name} Fans Speed' 'Fans' 'Rotations Per Minute (RPM)' '' line 6002 $sensors_update_every"
					echo >>$TMP_DIR/temp.sh "echo \"BEGIN sensors.fan_${id} \$1\""
					;;

				*)
					continue
					;;
			esac

			for x in $files
			do
				file="$x"
				fid="$( fixid "$file" )"
				lfile="$( basename $file | sed "s|_input$|_label|g" )"
				labelname="$( basename $file )"

				if [ ! "$path/$lfile" = "$file" -a -f "$path/$lfile" ]
					then
					labelname="$( cat "$path/$lfile" )"
				fi

				echo "DIMENSION $fid '$labelname' absolute $multiplier $divisor"
				echo >>$TMP_DIR/temp.sh "printf \"SET $fid = \"; cat $file "
			done

			echo >>$TMP_DIR/temp.sh "echo END"
		done
	done

	echo >>$TMP_DIR/temp.sh "}"
	. $TMP_DIR/temp.sh

	return 0
}

# _update is called continiously, to collect the values
sensors_update() {
	# the first argument to this function is the microseconds since last update
	# pass this parameter to the BEGIN statement (see bellow).

	# do all the work to collect / calculate the values
	# for each dimension
	# remember: KEEP IT SIMPLE AND SHORT

#	. $TMP_DIR/temp.sh $1

	return 0
}

