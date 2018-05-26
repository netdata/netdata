# no need for shebang - this file is included from other scripts

LOOPSLEEP_DATE="$(which date 2>/dev/null || command -v date 2>/dev/null)"
if [ -z "$LOOPSLEEP_DATE" ]
    then
    echo >&2 "$0: ERROR: Cannot find the command 'date' in the system path."
    exit 1
fi

# -----------------------------------------------------------------------------
# use the date command as a high resolution timer

now_ms=
LOOPSLEEPMS_HIGHRES=1
test "$($LOOPSLEEP_DATE +%N)" = "%N" && LOOPSLEEPMS_HIGHRES=0
test -z "$($LOOPSLEEP_DATE +%N)"     && LOOPSLEEPMS_HIGHRES=0
current_time_ms_from_date() {
    if [ $LOOPSLEEPMS_HIGHRES -eq 0 ]
    then
        now_ms="$($LOOPSLEEP_DATE +'%s')000"
    else
        now_ms="$(( $( $LOOPSLEEP_DATE +'%s * 1000 + %-N / 1000000' ) ))"
    fi
}

# -----------------------------------------------------------------------------
# use /proc/uptime as a high resolution timer

current_time_ms_from_date
current_time_ms_from_uptime_started="${now_ms}"
current_time_ms_from_uptime_last="${now_ms}"
current_time_ms_from_uptime_first=0
current_time_ms_from_uptime() {
    local up rest arr=() n

    read up rest </proc/uptime
    if [ $? -ne 0 ]
        then
        echo >&2 "$0: Cannot read /proc/uptime - falling back to current_time_ms_from_date()."
        current_time_ms="current_time_ms_from_date"
        current_time_ms_from_date
        current_time_ms_accuracy=1
        return
    fi

    arr=(${up//./ })

    if [ ${#arr[1]} -lt 1 ]
        then
        n="${arr[0]}000"
    elif [ ${#arr[1]} -lt 2 ]
        then
        n="${arr[0]}${arr[1]}00"
    elif [ ${#arr[1]} -lt 3 ]
        then
        n="${arr[0]}${arr[1]}0"
    else
        n="${arr[0]}${arr[1]}"
    fi

    now_ms=$((current_time_ms_from_uptime_started - current_time_ms_from_uptime_first + n))

    if [ "${now_ms}" -lt "${current_time_ms_from_uptime_last}" ]
        then
        echo >&2 "$0: Cannot use current_time_ms_from_uptime() - new time ${now_ms} is older than the last ${current_time_ms_from_uptime_last} - falling back to current_time_ms_from_date()."
        current_time_ms="current_time_ms_from_date"
        current_time_ms_from_date
        current_time_ms_accuracy=1
    fi

    current_time_ms_from_uptime_last="${now_ms}"
}
current_time_ms_from_uptime
current_time_ms_from_uptime_first="$((now_ms - current_time_ms_from_uptime_started))"
current_time_ms_from_uptime_last="${current_time_ms_from_uptime_first}"
current_time_ms="current_time_ms_from_uptime"
current_time_ms_accuracy=10
if [ "${current_time_ms_from_uptime_first}" -eq 0 ]
    then
    echo >&2 "$0: Invalid setup for current_time_ms_from_uptime() - falling back to current_time_ms_from_date()."
    current_time_ms="current_time_ms_from_date"
    current_time_ms_accuracy=1
fi

# -----------------------------------------------------------------------------
# use read with timeout for sleep

mysleep="mysleep_read"

mysleep_fifo="${NETDATA_CACHE_DIR-/tmp}/.netdata_bash_sleep_timer_fifo"
[ ! -e "${mysleep_fifo}" ] && mkfifo "${mysleep_fifo}"
[ ! -e "${mysleep_fifo}" ] && mysleep="sleep"

mysleep_read() {
    read -t "${1}" <>"${mysleep_fifo}"
    ret=$?
    if [ $ret -le 128 ]
        then
        echo >&2 "$0: Cannot use read for sleeping (return code ${ret})."
        mysleep="sleep"
        ${mysleep} "${1}"
    fi
}

# -----------------------------------------------------------------------------
# use bash loadable module for sleep

if [ "$((BASH_VERSINFO[0] +0))" -ge 3 ]
    then
    # enable modules only for bash version 3+

    for bash_modules_path in ${BASH_LOADABLES_PATH//:/ } "$(pkg-config bash --variable=loadablesdir 2>/dev/null)" "/usr/lib/bash" "/lib/bash" "/lib64/bash" "/usr/local/lib/bash" "/usr/local/lib64/bash"
    do
        [ -z "${bash_modules_path}" -o ! -d "${bash_modules_path}" ] && continue

        # check for sleep
        for bash_module_sleep in "sleep" "sleep.so"
        do
            if [ -f "${bash_modules_path}/${bash_module_sleep}" ]
                then
                if enable -f "${bash_modules_path}/${bash_module_sleep}" sleep 2>/dev/null
                    then
                    mysleep="builtin sleep"
                    #echo >&2 "$0: Using bash loadable ${bash_modules_path}/sleep for sleep"
                    break
                fi
            fi
        done

    done
fi


# -----------------------------------------------------------------------------
# this function is used to sleep a fraction of a second
# it calculates the difference between every time is called
# and tries to align the sleep time to give you exactly the
# loop you need.

LOOPSLEEPMS_LASTRUN=0
LOOPSLEEPMS_NEXTRUN=0
LOOPSLEEPMS_LASTSLEEP=0
LOOPSLEEPMS_LASTWORK=0

loopsleepms() {
    local tellwork=0 t="${1}" div s m now mstosleep

    if [ "${t}" = "tellwork" ]
    then
        tellwork=1
        shift
        t="${1}"
    fi

    # $t = the time in seconds to wait

    # if high resolution is not supported
    # just sleep the time requested, in seconds
    if [ ${LOOPSLEEPMS_HIGHRES} -eq 0 ]
    then
        sleep ${t}
        return
    fi

    # get the current time, in ms in ${now_ms}
    ${current_time_ms}

    # calculate ms since last run
    [ ${LOOPSLEEPMS_LASTRUN} -gt 0 ] && \
        LOOPSLEEPMS_LASTWORK=$((now_ms - LOOPSLEEPMS_LASTRUN - LOOPSLEEPMS_LASTSLEEP + current_time_ms_accuracy))
    # echo "# last loop's work took $LOOPSLEEPMS_LASTWORK ms"
    
    # remember this run
    LOOPSLEEPMS_LASTRUN=${now_ms}

    # calculate the next run
    LOOPSLEEPMS_NEXTRUN=$(( ( now_ms - ( now_ms % ( t * 1000 ) ) ) + ( t * 1000 ) ))

    # calculate ms to sleep
    mstosleep=$(( LOOPSLEEPMS_NEXTRUN - now_ms + current_time_ms_accuracy ))
    # echo "# mstosleep is $mstosleep ms"

    # if we are too slow, sleep some time
    test ${mstosleep} -lt 200 && mstosleep=200

    s=$(( mstosleep / 1000 ))
    m=$(( mstosleep - (s * 1000) ))
    [ "${m}" -lt 100 ] && m="0${m}"
    [ "${m}" -lt 10  ] && m="0${m}"

    test $tellwork -eq 1 && echo >&2 " >>> PERFORMANCE >>> WORK TOOK ${LOOPSLEEPMS_LASTWORK} ms ( $((LOOPSLEEPMS_LASTWORK * 100 / 1000)).$((LOOPSLEEPMS_LASTWORK % 10))% cpu ) >>> SLEEPING ${mstosleep} ms"

    # echo "# sleeping ${s}.${m}"
    # echo
    ${mysleep} ${s}.${m}

    # keep the values we need
    # for our next run
    LOOPSLEEPMS_LASTSLEEP=$mstosleep
}

# test it
#while [ 1 ]
#do
#    r=$(( (RANDOM * 2000 / 32767) ))
#    s=$((r / 1000))
#    m=$((r - (s * 1000)))
#    [ "${m}" -lt 100 ] && m="0${m}"
#    [ "${m}" -lt 10  ] && m="0${m}"
#    echo "${r} = ${s}.${m}"
#
#    # the work
#    ${mysleep} ${s}.${m}
#
#    # the alignment loop
#    loopsleepms tellwork 1
#done
