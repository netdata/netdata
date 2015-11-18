#!/bin/sh

# http://dev.mysql.com/doc/refman/5.0/en/server-status-variables.html
#
# https://dev.mysql.com/doc/refman/5.1/en/show-status.html
# SHOW STATUS provides server status information (see Section 5.1.6, “Server Status Variables”).
# This statement does not require any privilege.
# It requires only the ability to connect to the server.

mysql_update_every=5

declare -A mysql_cmds=() mysql_opts=() mysql_ids=()

mysql_get() {
	local ret
	
	"${@}" -s -e "show global status;"
	ret=$?

	[ $ret -ne 0 ] && echo "plugin_command_failure $ret"
	return $ret
}

mysql_check() {
	# this should return:
	#  - 0 to enable the chart
	#  - 1 to disable the chart

	local x m mysql_cmd

	require_cmd egrep || return 1
	require_cmd sed   || return 1

	[ -z "${mysql_cmd}" ] && mysql_cmd="$(which mysql)"

	if [ ${#mysql_opts[@]} -eq 0 ]
		then
		mysql_cmds[local]="$mysql_cmd"
		mysql_opts[local]=
	fi

	# check once if the url works
	for m in "${!mysql_opts[@]}"
	do
		[ -z "${mysql_cmds[$m]}" ] && mysql_cmds[$m]="$mysql_cmd"
		if [ -z "${mysql_cmds[$m]}" ]
			then
			echo >&2 "mysql: cannot get mysql command for '$m'. Please set mysql_cmds[$m]='/path/to/mysql', in $confd/mysql.conf"
		fi

		x="$(mysql_get "${mysql_cmds[$m]}" ${mysql_opts[$m]} | grep "^Connections[[:space:]]")"
		if [ ! $? -eq 0 -o -z "$x" ]
		then
			echo >&2 "mysql: cannot get global status for '$m'. Please set mysql_opts[$m]='options' to whatever needed to get connected to the mysql server, in $confd/mysql.conf"
			unset mysql_cmds[$m]
			unset mysql_opts[$m]
			unset mysql_ids[$m]
			continue
		fi

		mysql_ids[$m]="$( fixid "$m" )"
	done

	if [ ${#mysql_opts[@]} -eq 0 ]
		then
		echo >&2 "mysql: no mysql servers found. Please set mysql_opts[name]='options' to whatever needed to get connected to the mysql server, in $confd/mysql.conf"
	fi

	return 0
}

mysql_create() {
	local m

	# create the charts
	for m in "${mysql_ids[@]}"
	do
		cat <<EOF
CHART mysql_$m.bandwidth '' "mysql Bandwidth" "kilobits / sec" mysql_$m mysql area 20001 $mysql_update_every
DIMENSION Bytes_received in incremental 8 $((1024 * mysql_update_every))
DIMENSION Bytes_sent out incremental -8 $((1024 * mysql_update_every))

CHART mysql_$m.queries '' "mysql Queries" "queries / sec" mysql_$m mysql line 20002 $mysql_update_every
DIMENSION Queries queries incremental 1 $((1 * mysql_update_every))
DIMENSION Questions questions incremental 1 $((1 * mysql_update_every))
DIMENSION Slow_queries slow_queries incremental -1 $((1 * mysql_update_every))

CHART mysql_$m.operations '' "mysql Operations" "operations / sec" mysql_$m mysql line 20003 $mysql_update_every
DIMENSION Opened_tables opened_tables incremental 1 $((1 * mysql_update_every))
DIMENSION Flush_commands flush incremental 1 $((1 * mysql_update_every))
DIMENSION Handler_commit commit incremental 1 $((1 * mysql_update_every))
DIMENSION Handler_delete delete incremental 1 $((1 * mysql_update_every))
DIMENSION Handler_prepare prepare incremental 1 $((1 * mysql_update_every))
DIMENSION Handler_read_first read_first incremental 1 $((1 * mysql_update_every))
DIMENSION Handler_read_key read_key incremental 1 $((1 * mysql_update_every))
DIMENSION Handler_read_next read_next incremental 1 $((1 * mysql_update_every))
DIMENSION Handler_read_prev read_prev incremental 1 $((1 * mysql_update_every))
DIMENSION Handler_read_rnd read_rnd incremental 1 $((1 * mysql_update_every))
DIMENSION Handler_read_rnd_next read_rnd_next incremental 1 $((1 * mysql_update_every))
DIMENSION Handler_rollback rollback incremental 1 $((1 * mysql_update_every))
DIMENSION Handler_savepoint savepoint incremental 1 $((1 * mysql_update_every))
DIMENSION Handler_savepoint_rollback savepoint_rollback incremental 1 $((1 * mysql_update_every))
DIMENSION Handler_update update incremental 1 $((1 * mysql_update_every))
DIMENSION Handler_write write incremental 1 $((1 * mysql_update_every))

CHART mysql_$m.table_locks '' "mysql Tables Locks" "locks / sec" mysql_$m mysql line 20004 $mysql_update_every
DIMENSION Table_locks_immediate immediate incremental 1 $((1 * mysql_update_every))
DIMENSION Table_locks_waited waited incremental -1 $((1 * mysql_update_every))

CHART mysql_$m.select_issues '' "mysql Select Issues" "issues / sec" mysql_$m mysql line 20005 $mysql_update_every
DIMENSION Select_full_join full_join incremental 1 $((1 * mysql_update_every))
DIMENSION Select_full_range_join full_range_join incremental 1 $((1 * mysql_update_every))
DIMENSION Select_range range incremental 1 $((1 * mysql_update_every))
DIMENSION Select_range_check range_check incremental 1 $((1 * mysql_update_every))
DIMENSION Select_scan scan incremental 1 $((1 * mysql_update_every))

CHART mysql_$m.sort_issues '' "mysql Sort Issues" "issues / sec" mysql_$m mysql line 20006 $mysql_update_every
DIMENSION Sort_merge_passes merge_passes incremental 1 $((1 * mysql_update_every))
DIMENSION Sort_range range incremental 1 $((1 * mysql_update_every))
DIMENSION Sort_scan scan incremental 1 $((1 * mysql_update_every))
EOF
	done
	return 0
}


mysql_update() {
	# the first argument to this function is the microseconds since last update
	# pass this parameter to the BEGIN statement (see bellow).

	# do all the work to collect / calculate the values
	# for each dimension
	# remember: KEEP IT SIMPLE AND SHORT

	# 1. get the counters page from mysql
	# 2. sed to remove spaces; replace . with _; remove spaces around =; prepend each line with: local mysql_
	# 3. egrep lines starting with:
	#    local mysql_client_http_ then one or more of these a-z 0-9 _ then = and one of more of 0-9
	#    local mysql_server_all_ then one or more of these a-z 0-9 _ then = and one of more of 0-9
	# 4. then execute this as a script with the eval
	#
	# be very carefull with eval:
	# prepare the script and always grep at the end the lines that are usefull, so that
	# even if something goes wrong, no other code can be executed

	local m x
	for m in "${!mysql_ids[@]}"
	do
		x="${mysql_ids[$m]}"
		mysql_plugin_command_failure=0
		eval "$(mysql_get "${mysql_cmds[$m]}" ${mysql_opts[$m]} |\
			sed -e "s/[[:space:]]\+/ /g" -e "s/\./_/g" -e "s/^\([a-zA-Z0-9_]\+\)[[:space:]]\+\([0-9]\+\)$/local mysql_\1=\2/g" |\
			egrep "^local mysql_[a-zA-Z0-9_]+=[[:digit:]]+$")"

		if [ $mysql_plugin_command_failure -ne 0 ]
			then
			unset mysql_ids[$m]
			unset mysql_opts[$m]
			unset mysql_cmds[$m]
			echo >&2 "mysql: failed to get values for '$m', disabling it."
			continue
		fi

	# write the result of the work.
	cat <<VALUESEOF
BEGIN mysql_$x.bandwidth $1
SET Bytes_received = $mysql_Bytes_received
SET Bytes_sent = $mysql_Bytes_sent
END
BEGIN mysql_$x.queries $1
SET Queries = $mysql_Queries
SET Questions = $mysql_Questions
SET Slow_queries = $mysql_Slow_queries
END
BEGIN mysql_$x.operations $1
SET Opened_tables = $mysql_Opened_tables
SET Flush_commands = $mysql_Flush_commands
SET Handler_commit = $mysql_Handler_commit
SET Handler_delete = $mysql_Handler_delete
SET Handler_prepare = $mysql_Handler_prepare
SET Handler_read_first = $mysql_Handler_read_first
SET Handler_read_key = $mysql_Handler_read_key
SET Handler_read_next = $mysql_Handler_read_next
SET Handler_read_prev = $mysql_Handler_read_prev
SET Handler_read_rnd = $mysql_Handler_read_rnd
SET Handler_read_rnd_next = $mysql_Handler_read_rnd_next
SET Handler_rollback = $mysql_Handler_rollback
SET Handler_savepoint = $mysql_Handler_savepoint
SET Handler_savepoint_rollback = $mysql_Handler_savepoint_rollback
SET Handler_update = $mysql_Handler_update
SET Handler_write = $mysql_Handler_write
END
BEGIN mysql_$x.table_locks $1
SET Table_locks_immediate = $mysql_Table_locks_immediate
SET Table_locks_waited = $mysql_Table_locks_waited
END
BEGIN mysql_$x.select_issues $1
SET Select_full_join = $mysql_Select_full_join
SET Select_full_range_join = $mysql_Select_full_range_join
SET Select_range = $mysql_Select_range
SET Select_range_check = $mysql_Select_range_check
SET Select_scan = $mysql_Select_scan
END
BEGIN mysql_$x.sort_issues $1
SET Sort_merge_passes = $mysql_Sort_merge_passes
SET Sort_range = $mysql_Sort_range
SET Sort_scan = $mysql_Sort_scan
END
VALUESEOF
	done

	[ ${#mysql_ids[@]} -eq 0 ] && echo >&2 "mysql: no mysql servers left active." && return 1
	return 0
}

