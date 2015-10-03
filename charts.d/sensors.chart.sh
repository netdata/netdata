#!/bin/sh

# if this chart is called X.chart.sh, then all functions and global variables
# must start with X_

sensors_sys_dir="/sys/devices"
sensors_sys_depth=10

# _update_every is a special variable - it holds the number of seconds
# between the calls of the _update() function
sensors_update_every=

sensors_find_all_files() {
	find $1 -maxdepth $sensors_sys_depth -name \*_input -o -name temp 2>/dev/null
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

	[ ! -z "$( sensors_find_all_files $sensors_sys_dir )" ] && return 0
	return 1
}

sensors_check_files() {
	local f= v=
	for f in $*
	do
		echo >&2 "checking $f"
		v="$( cat $f )"
		v=$(( v + 1 - 1 ))
		[ $v -ne 0 ] && echo "$f"
	done
}

# _create is called once, to create the charts
sensors_create() {
	local path= dir= name= x= file= lfile= labelname= labelid= device= subsystem= id= type= mode= files= multiplier= divisor=

	echo >$TMP_DIR/temp.sh "sensors_update() {"

	for path in $( sensors_find_all_dirs $sensors_sys_dir | sort -u )
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

		echo >&2 "charts.d: sensors on path='$path', dir='$dir', device='$device', subsystem='$subsystem', id='$id', name='$name'"

		for mode in temperature voltage fans power current energy humidity
		do
			files=
			multiplier=1
			divisor=1
			algorithm="absolute"

			case $mode in
				temperature)
					files="$( ls $path/temp*_input 2>/dev/null; ls $path/temp 2>/dev/null )"
					files="$( sensors_check_files $files )"
					[ -z "$files" ] && continue
					echo "CHART sensors.temp_${id} '' '${name} Temperature' 'Celcius' '${device}' '' line 6000 $sensors_update_every"
					echo >>$TMP_DIR/temp.sh "echo \"BEGIN sensors.temp_${id} \$1\""
					divisor=1000
					;;

				voltage)
					files="$( ls $path/in*_input 2>/dev/null )"
					files="$( sensors_check_files $files )"
					[ -z "$files" ] && continue
					echo "CHART sensors.volt_${id} '' '${name} Voltage' 'Volts' '${device}' '' line 6001 $sensors_update_every"
					echo >>$TMP_DIR/temp.sh "echo \"BEGIN sensors.volt_${id} \$1\""
					divisor=1000
					;;

				current)
					files="$( ls $path/curr*_input 2>/dev/null )"
					files="$( sensors_check_files $files )"
					[ -z "$files" ] && continue
					echo "CHART sensors.curr_${id} '' '${name} Current' 'Ampere' '${device}' '' line 6002 $sensors_update_every"
					echo >>$TMP_DIR/temp.sh "echo \"BEGIN sensors.curr_${id} \$1\""
					divisor=1000
					;;

				power)
					files="$( ls $path/power*_input 2>/dev/null )"
					files="$( sensors_check_files $files )"
					[ -z "$files" ] && continue
					echo "CHART sensors.power_${id} '' '${name} Power' 'Watt' '${device}' '' line 6003 $sensors_update_every"
					echo >>$TMP_DIR/temp.sh "echo \"BEGIN sensors.power_${id} \$1\""
					divisor=1000000
					;;

				fans)
					files="$( ls $path/fan*_input 2>/dev/null )"
					files="$( sensors_check_files $files )"
					[ -z "$files" ] && continue
					echo "CHART sensors.fan_${id} '' '${name} Fans Speed' 'Rotations / Minute' '${device}' '' line 6004 $sensors_update_every"
					echo >>$TMP_DIR/temp.sh "echo \"BEGIN sensors.fan_${id} \$1\""
					;;

				emergy)
					files="$( ls $path/energy*_input 2>/dev/null )"
					files="$( sensors_check_files $files )"
					[ -z "$files" ] && continue
					echo "CHART sensors.energy_${id} '' '${name} Energy' 'Joule' '${device}' '' areastack 6005 $sensors_update_every"
					echo >>$TMP_DIR/temp.sh "echo \"BEGIN sensors.energy_${id} \$1\""
					algorithm="incremental"
					divisor=1000000
					;;

				humidity)
					files="$( ls $path/humidity*_input 2>/dev/null )"
					files="$( sensors_check_files $files )"
					[ -z "$files" ] && continue
					echo "CHART sensors.humidity_${id} '' '${name} Humidity' 'Percent' '${device}' '' line 6006 $sensors_update_every"
					echo >>$TMP_DIR/temp.sh "echo \"BEGIN sensors.humidity_${id} \$1\""
					divisor=1000
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
				labelname="$( basename $file | sed "s|_input$||g" )"

				if [ ! "$path/$lfile" = "$file" -a -f "$path/$lfile" ]
					then
					labelname="$( cat "$path/$lfile" )"
				fi

				echo "DIMENSION $fid '$labelname' $algorithm $multiplier $divisor"
				echo >>$TMP_DIR/temp.sh "printf \"SET $fid = \"; cat $file "
			done

			echo >>$TMP_DIR/temp.sh "echo END"
		done
	done

	echo >>$TMP_DIR/temp.sh "}"
	cat >&2 $TMP_DIR/temp.sh
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

