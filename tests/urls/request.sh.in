#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later

################################################################################################
####																						####
####									GLOBAL VARIABLES									####
####																						####
################################################################################################

# The current time
CT=$(date +'%s')

# The previous time
PT=$((CT - 30))

# The output directory where we will store the results and error
OUTDIR="tests"
OUTEDIR="encoded_tests"
OUTOPTDIR="options"
ERRDIR="etests"
NOCOLOR='\033[0'
RED='\033[0;31m'
GREEN='\033[0;32m'

################################################################################################
####																						####
####										FUNCTIONS										####
####																						####
################################################################################################

# Print error message and close script
netdata_print_error(){
    echo "${RED} Closing due error \"$1\" code \"$2\" ${NOCOLOR}"
    exit 1
}

# Print the header message of the function
netdata_print_header() {
    echo "$1"
}

# Create the main directory where the results will be stored
netdata_create_directory() {
    netdata_print_header "Creating directory $1"
    if [ ! -d "$1" ]; then
        mkdir "$1"
		TEST=$?
        if [ $TEST -ne  0 ]; then
            netdata_print_error "Cannot create directory  $?"
        fi
    else
        echo "Working with directory $OUTDIR"
    fi
}

#Check whether download did not have problem
netdata_test_download(){
    grep "HTTP/1.1 200 OK" "$1" 2>/dev/null 1>/dev/null
	TEST=$?
    if [ $TEST -ne 0 ]; then
        netdata_print_error "Cannot do download of the page $2" $?
        exit 1
    fi
}

#Check whether download had a problem
netdata_error_test(){
    grep "HTTP/1.1 200 OK" "$1" 2>/dev/null 1>/dev/null
	TEST=$?
    if [ $TEST -eq 0 ]; then
        netdata_print_error "The page $2 did not answer with an error" $?
        exit 1
    fi
}


# Download information from Netdata 
netdata_download_various() {
    netdata_print_header "Getting $2"
    curl -v -k --create-dirs -o "$OUTDIR/$3.out" "$1/$2" 2> "$OUTDIR/$3.err"
    netdata_test_download "$OUTDIR/$3.err" "$1/$2"
}

netdata_download_various_with_options() {
    netdata_print_header "Getting options for $2"
    curl -X OPTIONS -v -k --create-dirs -o "$OUTOPTDIR/$3.out" "$1/$2" 2> "$OUTOPTDIR/$3.err"
    netdata_test_download "$OUTOPTDIR/$3.err" "$1/$2"
}

# Download information from Netdata 
netdata_wrong_request_various() {
    netdata_print_header "Getting $2"
    curl -v -k --create-dirs -o "$ERRDIR/$3.out" "$1/$2" 2> "$ERRDIR/$3.err"
    netdata_error_test "$ERRDIR/$3.err" "$1/$2"
}

# Download charts from Netdata 
netdata_download_charts() {
    curl -v -k --create-dirs -o "$OUTDIR/charts.out" "$1/$2" 2> "$OUTDIR/charts.err"
    netdata_test_download "$OUTDIR/charts.err" "$1/$2"

    #Rewrite the next
    grep -w "id" tests/charts.out| cut -d: -f2 | grep "\"," | sed s/,//g | sort
}

#Test options for a specific chart
netdata_download_chart() {
    SEPARATOR="&"
    EQUAL="="
    OUTD=$OUTDIR
    ENCODED=" "
    for I in $(seq 0 1); do
        if [ "$I" -eq "1" ] ; then
            SEPARATOR="%26"
            EQUAL="%3D"
            OUTD=$OUTEDIR
            ENCODED="encoded"
        fi

		NAME=${3//\"/}
        netdata_print_header "Getting data for $NAME using $4 $ENCODED"

        LDIR=$OUTD"/"$4

        LURL="$1/$2$EQUAL$NAME"

        NAME=$NAME"_$4"

        curl -v -k --create-dirs -o "$LDIR/$NAME.out" "$LURL" 2> "$LDIR/$NAME.err"
        netdata_test_download "$LDIR/$NAME.err" "$LURL"

        UFILES=( "points" "before" "after" )
        COUNTER=0
        for OPT in "points=100" "before=$PT" "after=$CT" ;
        do
            LURL="$LURL$SEPARATOR$OPT"
            LFILE=$NAME"_${UFILES[$COUNTER]}";

            curl -v -k --create-dirs -o "$LDIR/$LFILE.out" "$LURL" 2> "$LDIR/$LFILE.err"
            netdata_test_download "$LDIR/$LFILE.err" "$LURL"

            COUNTER=$((COUNTER + 1))
        done

        LURL="$LURL&group$EQUAL"
        for OPT in "min" "max" "sum" "median" "stddev" "cv" "ses" "des" "incremental_sum" "average";
        do
            TURL=$LURL$OPT
            TFILE=$NAME"_$OPT";
            curl -v -k --create-dirs -o "$LDIR/$TFILE.out" "$TURL" 2> "$LDIR/$TFILE.err"
            netdata_test_download "$LDIR/$TFILE.err" "$TURL"
            for MORE in "jsonp" "json" "ssv" "csv" "datatable" "datasource" "tsv" "ssvcomma" "html" "array";
            do
                TURL=$TURL"&format="$MORE
                TFILE=$NAME"_$OPT""_$MORE";
                curl -v -k --create-dirs -o "$LDIR/$TFILE.out" "$TURL" 2> "$LDIR/$TFILE.err"
                netdata_test_download "$LDIR/$TFILE.err" "$TURL"
            done
        done

        LURL="$LURL$OPT&gtime=60"
        NFILE=$NAME"_gtime"
        curl -v -k --create-dirs -o "$LDIR/$NFILE.out" "$TURL" 2> "$LDIR/$NFILE.err"
        netdata_test_download "$LDIR/$NFILE.err" "$LURL"

        LURL="$LURL$OPT&options=percentage"
        NFILE=$NAME"_percentage"
        curl -v -k --create-dirs -o "$LDIR/$NFILE.out" "$TURL" 2> "$LDIR/$NFILE.err"
        netdata_test_download "$LDIR/$NFILE.err" "$LURL"

        LURL="$LURL$OPT&dimensions=system%7Cnice"
        NFILE=$NAME"_dimension"
        curl -v -k --create-dirs -o "$LDIR/$NFILE.out" "$TURL" 2> "$LDIR/$NFILE.err"
        netdata_test_download "$LDIR/$NFILE.err" "$LURL"

        LURL="$LURL$OPT&label=testing"
        NFILE=$NAME"_label"
        curl -v -k --create-dirs -o "$LDIR/$NFILE.out" "$TURL" 2> "$LDIR/$NFILE.err"
        netdata_test_download "$LDIR/$NFILE.err" "$LURL"
    done
}

# Download information from Netdata 
netdata_download_allmetrics() {
    netdata_print_header "Getting All metrics"
    LURL="$1/api/v1/allmetrics?format="
    for FMT in "shell" "prometheus" "prometheus_all_hosts" "json" ;
    do
        TURL=$LURL$FMT
        for OPT in "yes" "no";
        do
            if [ "$FMT" == "prometheus" ]; then
                TURL="$TURL&help=$OPT&types=$OPT&timestamps=$OPT"
            fi
            TURL="$TURL&names=$OPT&oldunits=$OPT&hideunits=$OPT&prefix=ND"

            NAME="allmetrics_$FMT"
            echo "$OUTDIR/$2/$NAME.out"
            curl -v -k --create-dirs -o "$OUTDIR/$2/$NAME.out" "$TURL" 2> "$OUTDIR/$2/$NAME.err"
            netdata_test_download "$OUTDIR/$2/$NAME.err" "$TURL"
            done
    done
}


####################################################
####																						####
####									MAIN ROUTINE              ####
####																						####
####################################################
MURL="http://127.0.0.1:19999"

if [ -n "$1" ]; then
    MURL="$1"
fi

netdata_create_directory $OUTDIR
netdata_create_directory $OUTEDIR
netdata_create_directory $OUTOPTDIR
netdata_create_directory $ERRDIR

wget --no-check-certificate --execute="robots = off" --mirror --convert-links --no-parent "$MURL"
TEST=$?
if [ $TEST -ne "0" ] ; then
    echo "Cannot connect to Netdata"
    exit 1
fi

netdata_download_various "$MURL" "netdata.conf" "netdata.conf"

netdata_download_various_with_options "$MURL" "netdata.conf" "netdata.conf"

netdata_wrong_request_various "$MURL" "api/v15/info?this%20could%20not%20be%20here" "err_version"

netdata_wrong_request_various "$MURL" "api/v1/\(*@&$\!$%%5E\)\!$*%&\)\!$*%%5E*\!%5E%\!%5E$%\!%5E%\(\!*%5E*%5E%\(*@&$%5E%\(\!%5E#*&\!^#$*&\!^%\)@\($%^\)\!*&^\(\!*&^#$&#$\)\!$%^\)\!$*%&\)#$\!^#*$^\!\(*#^#\)\!%^\!\)$*%&\!\(*&$\!^#$*&^\!*#^$\!*^\)%\(\!*&$%\)\(\!&#$\!^*#&$^\!*^%\)\!$%\)\!\(&#$\!^#*&^$" "err_version2"

netdata_download_various "$MURL" "api/v1/info" "info"
netdata_download_various_with_options "$MURL" "api/v1/info" "info"
netdata_download_various "$MURL" "api/v1/info?this%20could%20not%20be%20here" "err_info"

netdata_print_header "Getting all the netdata charts"
CHARTS=$( netdata_download_charts "$MURL" "api/v1/charts" )
WCHARTS=$( netdata_download_charts "$MURL" "api/v1/charts?this%20could%20not%20be%20here" )
WCHARTS2=$( netdata_download_charts "$MURL" "api/v1/charts%3fthis%20could%20not%20be%20here" )

if [ ${#CHARTS[@]} -ne ${#WCHARTS[@]} ]; then
    echo "The number of charts does not match with division not encoded.";
    exit 2;
elif [ ${#CHARTS[@]} -ne ${#WCHARTS2[@]} ]; then
    echo "The number of charts does not match when everything is encoded";
    exit 3;
fi

netdata_wrong_request_various "$MURL" "api/v1/chart" "err_chart_without_chart"
netdata_wrong_request_various "$MURL" "api/v1/chart?_=234231424242" "err_chart_arg"

netdata_download_various "$MURL" "api/v1/chart?chart=cpu.cpu0_interrupts&_=234231424242" "chart_cpu_with_more_args"
netdata_download_various_with_options "$MURL" "api/v1/chart?chart=cpu.cpu0_interrupts&_=234231424242" "chart_cpu_with_more_args"

netdata_download_various "$MURL" "api/v1/chart%3Fchart=cpu.cpu0_interrupts&_=234231424242" "chart_cpu_with_more_args_encoded"
netdata_download_various_with_options "$MURL" "api/v1/chart%3Fchart=cpu.cpu0_interrupts&_=234231424242" "chart_cpu_with_more_args_encoded"
netdata_download_various "$MURL" "api/v1/chart%3Fchart=cpu.cpu0_interrupts%26_=234231424242" "chart_cpu_with_more_args_encoded2"
netdata_download_various "$MURL" "api/v1/chart%3Fchart%3Dcpu.cpu0_interrupts%26_%3D234231424242" "chart_cpu_with_more_args_encoded3"

netdata_create_directory "$OUTDIR/chart"
for I in $CHARTS ; do
	NAME=${I//\"/}
    netdata_download_various "$MURL" "api/v1/chart?chart=$NAME"  "chart/$NAME"
done

netdata_wrong_request_various "$MURL" "api/v1/alarm_variables" "err_alarm_variables_without_chart"
netdata_wrong_request_various "$MURL" "api/v1/alarm_variables?_=234231424242" "err_alarm_variables_arg"
netdata_download_various "$MURL" "api/v1/alarm_variables?chart=cpu.cpu0_interrupts&_=234231424242" "alarm_cpu_with_more_args"

netdata_create_directory "$OUTDIR/alarm_variables"
for I in $CHARTS ; do
	NAME=${I//\"/}
    netdata_download_various "$MURL" "api/v1/alarm_variables?chart=$NAME"  "alarm_variables/$NAME"
done

netdata_create_directory "$OUTDIR/badge"
netdata_create_directory "$OUTEDIR/badge"
for I in $CHARTS ; do
    netdata_download_chart "$MURL" "api/v1/badge.svg?chart" "$I" "badge"
done

netdata_create_directory "$OUTDIR/allmetrics"
netdata_download_allmetrics "$MURL" "allmetrics"

netdata_download_various "$MURL" "api/v1/alarms?all"  "alarms_all"
netdata_download_various "$MURL" "api/v1/alarms?active"  "alarms_active"
netdata_download_various "$MURL" "api/v1/alarms"  "alarms_nothing"

netdata_download_various "$MURL" "api/v1/alarm_log?after"  "alarm_without"
netdata_download_various "$MURL" "api/v1/alarm_log"  "alarm_nothing"
netdata_download_various "$MURL" "api/v1/alarm_log?after&_=$PT"  "alarm_log"

netdata_create_directory "$OUTDIR/data"
netdata_create_directory "$OUTEDIR/data"
for I in $CHARTS ; do
    netdata_download_chart "$MURL" "api/v1/data?chart" "$I" "data"
    break;
done

echo -e "${GREEN}ALL the URLS got 200 as answer! ${NOCOLOR}"

exit 0
