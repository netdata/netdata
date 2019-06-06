#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later

################################################################################################
####																						####
####									GLOBAL VARIABLES									####
####																						####
################################################################################################

NETDATA_VARLIB_DIR="@varlibdir_POST@"

#CT=`date +'%s%N' |cut -b1-13`
# The current time
CT=`date +'%s'`

# The previous time
PT=$(( $CT - 100))

# The curl options used to do download
CURLOPTS="-v --create-dirs -o"

# The output directory where we will store the results and error
OUTDIR="tests"

################################################################################################
####																						####
####										FUNCTIONS										####
####																						####
################################################################################################

# Print error message and close script
netdata_print_error(){
	echo "Closing due error \"$1\" code \"$2\""
	exit 1
}

# Print the header message of the function
netdata_print_header() {
	echo "$1"
}

# Create the main directory where the results will be stored
netdata_create_directory() {
	netdata_print_header "Creating directory $1"
	if [ ! -d $1 ]; then
		mkdir $1
		if [ $? -ne  0 ]; then
			netdata_print_error "Cannot create directory" $?
		fi
	else
		echo "Working with directory $OUTDIR"
	fi
}

netdata_test_download(){
	grep "HTTP/1.1 200 OK" $1 2>/dev/null 1>/dev/null
	if [ $? -ne 0 ]; then
		netdata_print_error "Cannot do download of the page $2" $?
	fi
}

# Download information from Netdata 
netdata_download_various() {
	netdata_print_header "Getting $2"
	curl $CURLOPTS $OUTDIR/$3.out "$1/$2" 2> $OUTDIR/$3.err
	netdata_test_download $OUTDIR/$3.err "$1/$2"
}

# Download charts from Netdata 
netdata_download_charts() {
	curl $CURLOPTS $OUTDIR/charts.out "$1/$2/charts" 2> $OUTDIR/charts.err
	netdata_test_download $OUTDIR/charts.err "$1/$2/charts"

	#Rewrite the next
	cat tests/charts.out | grep -w "id"| cut -d: -f2 | grep "\"," | sed s/,//g | sort 
}

#Test options for a specific chart
netdata_download_chart() {
	NAME=`echo $3| sed s/\"//g`
	netdata_print_header "Getting data for $NAME using $4"

	LDIR=$OUTDIR"/"$4

	LURL=$1/$2=$NAME

	NAME=$NAME"_$4"

	curl $CURLOPTS $LDIR/$NAME.out "$LURL" 2> $LDIR/$NAME.err
	netdata_test_download $LDIR/$NAME.err $LURL

	UFILES=( "points" "before" "after" )
	COUNTER=0
	for OPT in "&points=100" "&before=$PT" "&after=$CT" ;
	do
		LURL="$LURL$OPT"
		LFILE=$NAME"_${UFILES[$COUNTER]}";

		curl $CURLOPTS "$LDIR/$LFILE.out" "$LURL" 2> "$LDIR/$LFILE.err"
		netdata_test_download $LDIR/$LFILE.err $LURL

		COUNTER=$(($COUNTER + 1))
	done

	LURL="$LURL&group="
	for OPT in "min" "max" "sum" "median" "stddev" "cv" "ses" "des" "incremental_sum" "average";
	do
		TURL=$LURL$OPT
		TFILE=$NAME"_$OPT";
		curl $CURLOPTS "$LDIR/$TFILE.out" "$TURL" 2> "$LDIR/$TFILE.err"
		netdata_test_download $LDIR/$TFILE.err $TURL
		for MORE in "jsonp" "json" "ssv" "csv" "datatable" "datasource" "tsv" "ssvcomma" "html" "array";
		do
			TURL=$TURL"&format="$MORE
			TFILE=$NAME"_$OPT""_$MORE";
			curl $CURLOPTS "$LDIR/$TFILE.out" "$TURL" 2> "$LDIR/$TFILE.err"
			netdata_test_download $LDIR/$TFILE.err $TURL
		done

	done

	LURL="$LURL$OPT&gtime=60"
	NFILE=$NAME"_gtime"
	curl $CURLOPTS "$LDIR/$NFILE.out" "$TURL" 2> "$LDIR/$NFILE.err"
	netdata_test_download $LDIR/$NFILE.err $LURL

	LURL="$LURL$OPT&options=percentage"
	NFILE=$NAME"_percentage"
	curl $CURLOPTS "$LDIR/$NFILE.out" "$TURL" 2> "$LDIR/$NFILE.err"
	netdata_test_download $LDIR/$NFILE.err $LURL

	LURL="$LURL$OPT&options=percentage"
	NFILE=$NAME"_percentage"
	curl $CURLOPTS "$LDIR/$NFILE.out" "$TURL" 2> "$LDIR/$NFILE.err"
	netdata_test_download $LDIR/$NFILE.err $LURL

	LURL="$LURL$OPT&dimensions=system%7Cnice"
	NFILE=$NAME"_dimension"
	curl $CURLOPTS "$LDIR/$NFILE.out" "$TURL" 2> "$LDIR/$NFILE.err"
	netdata_test_download $LDIR/$NFILE.err $LURL

}

# Download information from Netdata 
netdata_download_allmetrics() {
	netdata_print_header "Getting All metrics"
	curl $CURLOPTS $OUTDIR/allmetrics.out "$1/$2" 2> $OUTDIR/allmetrics.err
	netdata_test_download $OUTDIR/allmetrics.err "$1/$2"
}

# Download charts from Netdata 

################################################################################################
####																						####
####									MAIN ROUTINE										####
####																						####
################################################################################################
MURL="http://127.0.0.1:19999"

wget --execute="robots = off" --mirror --convert-links --no-parent http://127.0.0.1:19999

netdata_create_directory $OUTDIR

netdata_download_various $MURL "netdata.conf" "netdata.conf"

netdata_download_various $MURL "api/v1/info" "info"

netdata_download_various $MURL "api/v1/registry?action=hello"  "action"

netdata_print_header "Getting all the netdata charts"
CHARTS=$( netdata_download_charts "http://127.0.0.1:19999" "api/v1" )

netdata_download_various $MURL "api/v1/allmetrics?format=json"  "allmetrics"

netdata_download_various $MURL "api/v1/alarms?all"  "alarms_all"

netdata_download_various $MURL "api/v1/alarms?active"  "alarms_active"

netdata_download_various $MURL "api/v1/alarm_log?after&_=$PT"  "alarm_log"

for I in $CHARTS ; do
	NAME=`echo $I| sed s/\"//g`
	netdata_download_various $MURL "api/v1/alarm_variables?chart=$NAME"  "alarm_variables_$NAME"
done

netdata_create_directory "$OUTDIR/data"
for I in $CHARTS ; do
	netdata_download_chart $MURL "api/v1/data?chart" $I "data"
done

netdata_create_directory "$OUTDIR/badge.svg"
for I in $CHARTS ; do
	netdata_download_chart $MURL "api/v1/badge.svg?chart" $I "badge.svg"
done

if [ -f "${NETDATA_VARLIB_DIR}/netdata.api.key" ] ;then
	read -r CORRECT_TOKEN < "${NETDATA_VARLIB_DIR}/netdata.api.key"
else
	echo "${NETDATA_VARLIB_DIR}/netdata.api.key not found"
	echo "Token not found."
	exit 2
fi
curl -H "X-Auth-Token: $TOKEN" "http://127.0.0.1:19999/api/v1/manage/health?cmd=RESET"

echo "ALL the URLS got 200 as answer!"

exit 0
