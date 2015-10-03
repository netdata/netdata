#!/bin/sh

# if this chart is called X.chart.sh, then all functions and global variables
# must start with X_

# _update_every is a special variable - it holds the number of seconds
# between the calls of the _update() function
temp_update_every=

temp_find_all_files() {
	find $1 -name temp\?_input -o -name temp 2>/dev/null
}

temp_find_all_dirs() {
	temp_find_all_files $1 | while read
	do
		dirname $REPLY
	done | sort -u
}

# _check is called once, to find out if this chart should be enabled or not
temp_check() {

	# this should return:
	#  - 0 to enable the chart
	#  - 1 to disable the chart

	[ ! -z "$( temp_find_all_files /sys/devices/ )" ] && return 0
	return 1
}

# _create is called once, to create the charts
temp_create() {
	local path= dir= name= x= file= lfile= labelname= labelid= device= subsystem= id= type=

	echo >$TMP_DIR/temp.sh "temp_update() {"

	for path in $( temp_find_all_dirs /sys/devices/ | sort -u )
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

		echo "CHART temperature.${id} '' '${name} Temperature' 'Temperature' 'Celcius Degrees' '' line 6000 $temp_update_every"
		echo >>$TMP_DIR/temp.sh "echo \"BEGIN temperature.${id} \$1\""

		for x in $( temp_find_all_files $path | sort -u )
		do
			file="$x"
			fid="$( fixid "$file" )"
			lfile="$( basename $file | sed "s|_input$|_label|g" )"
			labelname="$( basename $file )"

			if [ ! "$path/$lfile" = "$file" -a -f "$path/$lfile" ]
				then
				labelname="$( cat "$path/$lfile" )"
			fi

			echo "DIMENSION $fid '$labelname' absolute 1 1000"
			echo >>$TMP_DIR/temp.sh "printf \"SET $fid = \"; cat $file "
			
			# echo >&2 "charts.d: temperature sensor on file='$file', label='$labelname'"
		done

		echo >>$TMP_DIR/temp.sh "echo END"
	done

	echo >>$TMP_DIR/temp.sh "}"
	. $TMP_DIR/temp.sh

	return 0
}

# _update is called continiously, to collect the values
temp_update() {
	# the first argument to this function is the microseconds since last update
	# pass this parameter to the BEGIN statement (see bellow).

	# do all the work to collect / calculate the values
	# for each dimension
	# remember: KEEP IT SIMPLE AND SHORT

#	. $TMP_DIR/temp.sh $1

	return 0
}

