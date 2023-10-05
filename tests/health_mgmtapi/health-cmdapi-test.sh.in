#!/usr/bin/env bash
# shellcheck disable=SC1117,SC2034,SC2059,SC2086,SC2181

NETDATA_VARLIB_DIR="@varlibdir_POST@"

check () {
	sec=1
	echo -e "  ${GRAY}Check: '${1}' in $sec sec"
	sleep $sec
    number=$RANDOM
	resp=$(curl -s "http://$URL/api/v1/alarms?all&$number")
	r=$(echo "${resp}" | \
	python3 -c "import sys, json; d=json.load(sys.stdin); \
	print(\
	d['alarms']['system.cpu.10min_cpu_usage']['disabled'], \
	d['alarms']['system.cpu.10min_cpu_usage']['silenced'] , \
	d['alarms']['system.cpu.10min_cpu_iowait']['disabled'], \
	d['alarms']['system.cpu.10min_cpu_iowait']['silenced'], \
	d['alarms']['system.load.load_trigger']['disabled'], \
	d['alarms']['system.load.load_trigger']['silenced'], \
	);" 2>&1)
	if [ $? -ne 0 ] ; then
		echo -e "  ${RED}ERROR: Unexpected response stored in /tmp/resp-$number.json"
		echo "$resp" > /tmp/resp-$number.json
		err=$((err+1))
		iter=0
	elif [ "${r}" != "${2}" ] ; then
		echo -e "  ${GRAY}WARNING: 'Got ${r}'. Expected '${2}'"
		iter=$((iter+1))
		if [ $iter -lt 10 ] ; then
			echo -e "  ${GRAY}Repeating test "
			check "$1" "$2"
		else
			echo -e "  ${RED}ERROR: 'Got ${r}'. Expected '${2}'"
			iter=0
			err=$((err+1))
		fi
	else
		echo -e "  ${GREEN}Success"
		iter=0
	fi
}

cmd () {
	echo -e "${WHITE}Cmd '${1}'"
	echo -en "  ${GRAY}Expecting '${2}' : "
	RESPONSE=$(curl -s "http://$URL/api/v1/manage/health?${1}" -H "X-Auth-Token: $TOKEN" 2>&1)
	if [ "${RESPONSE}" != "${2}" ] ; then
		echo -e "${RED}ERROR: Response '${RESPONSE}'"
		err=$((err+1))
	else
		echo -e "${GREEN}Success"
	fi
}

check_list() {
    RESPONSE=$(curl -s "http://$URL/api/v1/manage/health?cmd=LIST" -H "X-Auth-Token: $TOKEN" 2>&1)

    NAME="$1-list.json"
    echo $RESPONSE > $NAME
    diff $NAME expected_list/$NAME 1>/dev/null 2>&1
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}Success: The list command got the correct answer for $NAME!"
    else
        echo -e "${RED}ERROR: the files $NAME and expected_list/$NAME does not match."
        exit 1
    fi
}

WHITE='\033[0;37m'
RED='\033[0;31m'
GREEN='\033[0;32m'
GRAY='\033[0;37m'

SETUP=0
RESTART=0
CLEANUP=0
TEST=0
URL="localhost:19999"

err=0


	HEALTH_CMDAPI_MSG_AUTHERROR="Auth Error"
	HEALTH_CMDAPI_MSG_SILENCEALL="All alarm notifications are silenced"
	HEALTH_CMDAPI_MSG_DISABLEALL="All health checks are disabled"
	HEALTH_CMDAPI_MSG_RESET="All health checks and notifications are enabled"
	HEALTH_CMDAPI_MSG_DISABLE="Health checks disabled for alarms matching the selectors"
	HEALTH_CMDAPI_MSG_SILENCE="Alarm notifications silenced for alarms matching the selectors"
	HEALTH_CMDAPI_MSG_ADDED="Alarm selector added"
	HEALTH_CMDAPI_MSG_INVALID_KEY="Invalid key. Ignoring it."
	HEALTH_CMDAPI_MSG_STYPEWARNING="WARNING: Added alarm selector to silence/disable alarms without a SILENCE or DISABLE command."
	HEALTH_CMDAPI_MSG_NOSELECTORWARNING="WARNING: SILENCE or DISABLE command is ineffective without defining any alarm selectors."

	if [ -f "${NETDATA_VARLIB_DIR}/netdata.api.key" ] ;then
		read -r CORRECT_TOKEN < "${NETDATA_VARLIB_DIR}/netdata.api.key"
	else
		echo "${NETDATA_VARLIB_DIR}/netdata.api.key not found"
		exit 1
	fi
	# Set correct token
	TOKEN="${CORRECT_TOKEN}"

	# Test default state
	cmd "cmd=RESET" "$HEALTH_CMDAPI_MSG_RESET"
	check "Default State" "False False False False False False"
	check_list "RESET"

	# Test auth failure
	TOKEN="Wrong token"
	cmd "cmd=DISABLE ALL" "$HEALTH_CMDAPI_MSG_AUTHERROR"
	check "Default State" "False False False False False False"
	check_list "DISABLE_ALL_ERROR"

	# Set correct token
	TOKEN="${CORRECT_TOKEN}"

	# Test disable
	cmd "cmd=DISABLE ALL" "$HEALTH_CMDAPI_MSG_DISABLEALL"
	check "All disabled" "True False True False True False"
	check_list "DISABLE_ALL"

	# Reset
	cmd "cmd=RESET" "$HEALTH_CMDAPI_MSG_RESET"
	check "Default State" "False False False False False False"
	check_list "RESET"

	# Test silence
	cmd "cmd=SILENCE ALL" "$HEALTH_CMDAPI_MSG_SILENCEALL"
	check "All silenced" "False True False True False True"
	check_list "SILENCE_ALL"

	# Reset
	cmd "cmd=RESET" "$HEALTH_CMDAPI_MSG_RESET"
	check "Default State" "False False False False False False"
	check_list "RESET"

	# Add silencer by name
	printf -v resp "$HEALTH_CMDAPI_MSG_SILENCE\n$HEALTH_CMDAPI_MSG_ADDED"
	cmd "cmd=SILENCE&alarm=*10min_cpu_usage *load_trigger" "${resp}"
	check "Silence notifications for alarm1 and load_trigger" "False True False False False True"
	check_list "SILENCE_ALARM_CPU_USAGE_LOAD_TRIGGER"

	# Convert to disable health checks
	cmd "cmd=DISABLE" "$HEALTH_CMDAPI_MSG_DISABLE"
	check "Disable notifications for alarm1 and load_trigger" "True False False False True False"
	check_list "DISABLE"

	# Convert back to silence notifications
	cmd "cmd=SILENCE" "$HEALTH_CMDAPI_MSG_SILENCE"
	check "Silence notifications for alarm1 and load_trigger" "False True False False False True"
	check_list "SILENCE"

	# Add second silencer by name
	cmd "alarm=*10min_cpu_iowait" "$HEALTH_CMDAPI_MSG_ADDED"
	check "Silence notifications for alarm1,alarm2 and load_trigger" "False True False True False True"
	check_list "ALARM_CPU_IOWAIT"

	# Reset
	cmd "cmd=RESET" "$HEALTH_CMDAPI_MSG_RESET"
	check_list "RESET"

	# Add silencer by chart
	printf -v resp "$HEALTH_CMDAPI_MSG_DISABLE\n$HEALTH_CMDAPI_MSG_ADDED"
	cmd "cmd=DISABLE&chart=system.load" "${resp}"
	check "Default State" "False False False False True False"
	check_list "DISABLE_SYSTEM_LOAD"

	# Add silencer by context
	cmd "context=system.cpu" "$HEALTH_CMDAPI_MSG_ADDED"
	check "Default State" "True False True False True False"
	check_list "CONTEXT_SYSTEM_CPU"

	# Reset
	cmd "cmd=RESET" "$HEALTH_CMDAPI_MSG_RESET"
	check_list "RESET"

	# Add second condition to a selector (AND)
	printf -v resp "$HEALTH_CMDAPI_MSG_SILENCE\n$HEALTH_CMDAPI_MSG_ADDED"
	cmd "cmd=SILENCE&alarm=*10min_cpu_usage *load_trigger&chart=system.load" "${resp}"
	check "Silence notifications load_trigger" "False False False False False True"
	check_list "SILENCE_ALARM_CPU_USAGE"

	# Add second selector with two conditions
	cmd "alarm=*10min_cpu_usage *load_trigger&context=system.cpu" "$HEALTH_CMDAPI_MSG_ADDED"
	check "Silence notifications load_trigger" "False True False False False True"
	check_list "ALARM_CPU_USAGE"

	# Reset
	cmd "cmd=RESET" "$HEALTH_CMDAPI_MSG_RESET"
	check_list "RESET"

	# Add silence command
	cmd "cmd=SILENCE" "$HEALTH_CMDAPI_MSG_SILENCE"
	check "Silence family load" "False False False False False True"
	check_list "SILENCE_2"

	# Reset
	cmd "cmd=RESET" "$HEALTH_CMDAPI_MSG_RESET"
	check_list "RESET"

	# Add command without silencers
	printf -v resp "$HEALTH_CMDAPI_MSG_SILENCE\n$HEALTH_CMDAPI_MSG_NOSELECTORWARNING"
	cmd "cmd=SILENCE" "${resp}"
	check "Command with no selector" "False False False False False False"
	check_list "SILENCE_3"

	# Add hosts silencer
	cmd "hosts=*" "$HEALTH_CMDAPI_MSG_ADDED"
	check "Silence all hosts" "False True False True False True"
	check_list "HOSTS"

	# Reset
	cmd "cmd=RESET" "$HEALTH_CMDAPI_MSG_RESET"
	check_list "RESET"

if [ $err -gt 0 ] ; then
	echo "$err error(s) found"
	exit 1
fi
