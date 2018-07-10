# no need for shebang - this file is loaded from charts.d.plugin
# SPDX-License-Identifier: GPL-3.0+

# netdata
# real-time performance and health monitoring, done right!
# (C) 2016 Costa Tsaousis <costa@tsaousis.gr>
#

# http://dev.mysql.com/doc/refman/5.0/en/server-status-variables.html
#
# https://dev.mysql.com/doc/refman/5.1/en/show-status.html
# SHOW STATUS provides server status information (see Section 5.1.6, “Server Status Variables”).
# This statement does not require any privilege.
# It requires only the ability to connect to the server.

mysql_update_every=2
mysql_priority=60000

declare -A mysql_cmds=() mysql_opts=() mysql_ids=() mysql_data=()

mysql_get() {
	local arr
	local oIFS="${IFS}"
	mysql_data=()
	IFS=$'\t'$'\n'
	#arr=($(run "${@}" -e "SHOW GLOBAL STATUS WHERE value REGEXP '^[0-9]';" | egrep "^(Bytes|Slow_|Que|Handl|Table|Selec|Sort_|Creat|Conne|Abort|Binlo|Threa|Innod|Qcach|Key_|Open)" ))
	#arr=($(run "${@}" -N -e "SHOW GLOBAL STATUS;" | egrep "^(Bytes|Slow_|Que|Handl|Table|Selec|Sort_|Creat|Conne|Abort|Binlo|Threa|Innod|Qcach|Key_|Open)[^ ]+\s[0-9]" ))
	arr=($(run "${@}" -N -e "SHOW GLOBAL STATUS;" | egrep "^(Bytes|Slow_|Que|Handl|Table|Selec|Sort_|Creat|Conne|Abort|Binlo|Threa|Innod|Qcach|Key_|Open)[^[:space:]]+[[:space:]]+[0-9]+" ))
	IFS="${oIFS}"

	[ "${#arr[@]}" -lt 3 ] && return 1
	local end=${#arr[@]}
	for ((i=2;i<end;i+=2)); do
		mysql_data["${arr[$i]}"]=${arr[$i+1]}
	done

	[ -z "${mysql_data[Connections]}" ] && return 1

	mysql_data[Thread_cache_misses]=0
	[ $(( mysql_data[Connections] + 1 - 1 )) -gt 0 ] && mysql_data[Thread_cache_misses]=$(( mysql_data[Threads_created] * 10000 / mysql_data[Connections] ))

	return 0
}

mysql_check() {
	# this should return:
	#  - 0 to enable the chart
	#  - 1 to disable the chart

	local x m mysql_cmd tryroot=0 unconfigured=0

	if [ "${1}" = "tryroot" ]
		then
		tryroot=1
		shift
	fi

	[ -z "${mysql_cmd}" ] && mysql_cmd="$(which mysql 2>/dev/null || command -v mysql 2>/dev/null)"

	if [ ${#mysql_opts[@]} -eq 0 ]
		then
		unconfigured=1

		mysql_cmds[local]="$mysql_cmd"

		if [ $tryroot -eq 1 ]
			then
			# the user has not configured us for mysql access
			# if the root user is passwordless in mysql, we can
			# attempt to connect to mysql as root
			mysql_opts[local]="-u root"
		else
			mysql_opts[local]=
		fi
	fi

	# check once if the url works
	for m in "${!mysql_opts[@]}"
	do
		[ -z "${mysql_cmds[$m]}" ] && mysql_cmds[$m]="$mysql_cmd"
		if [ -z "${mysql_cmds[$m]}" ]
			then
			error "cannot get mysql command for '$m'. Please set mysql_cmds[$m]='/path/to/mysql', in $confd/mysql.conf"
		fi

		mysql_get "${mysql_cmds[$m]}" ${mysql_opts[$m]}
		if [ ! $? -eq 0 ]
		then
			error "cannot get global status for '$m'. Please set mysql_opts[$m]='options' to whatever needed to get connected to the mysql server, in $confd/mysql.conf"
			unset mysql_cmds[$m]
			unset mysql_opts[$m]
			unset mysql_ids[$m]
			continue
		fi

		mysql_ids[$m]="$( fixid "$m" )"
	done

	if [ ${#mysql_opts[@]} -eq 0 ]
		then
		if [ ${unconfigured} -eq 1 -a ${tryroot} -eq 0 ]
			then
			mysql_check tryroot "${@}"
			return $?
		else
			error "no mysql servers found. Please set mysql_opts[name]='options' to whatever needed to get connected to the mysql server, in $confd/mysql.conf"
			return 1
		fi
	fi

	return 0
}

mysql_create() {
	local x

	# create the charts
	for x in "${mysql_ids[@]}"
	do
		cat <<EOF
CHART mysql_$x.net '' "mysql Bandwidth" "kilobits/s" bandwidth mysql.net area $((mysql_priority + 1)) $mysql_update_every
DIMENSION Bytes_received in incremental 8 1024
DIMENSION Bytes_sent out incremental -8 1024

CHART mysql_$x.queries '' "mysql Queries" "queries/s" queries mysql.queries line $((mysql_priority + 2)) $mysql_update_every
DIMENSION Queries queries incremental 1 1
DIMENSION Questions questions incremental 1 1
DIMENSION Slow_queries slow_queries incremental -1 1

CHART mysql_$x.handlers '' "mysql Handlers" "handlers/s" handlers mysql.handlers line $((mysql_priority + 3)) $mysql_update_every
DIMENSION Handler_commit commit incremental 1 1
DIMENSION Handler_delete delete incremental 1 1
DIMENSION Handler_prepare prepare incremental 1 1
DIMENSION Handler_read_first read_first incremental 1 1
DIMENSION Handler_read_key read_key incremental 1 1
DIMENSION Handler_read_next read_next incremental 1 1
DIMENSION Handler_read_prev read_prev incremental 1 1
DIMENSION Handler_read_rnd read_rnd incremental 1 1
DIMENSION Handler_read_rnd_next read_rnd_next incremental 1 1
DIMENSION Handler_rollback rollback incremental 1 1
DIMENSION Handler_savepoint savepoint incremental 1 1
DIMENSION Handler_savepoint_rollback savepoint_rollback incremental 1 1
DIMENSION Handler_update update incremental 1 1
DIMENSION Handler_write write incremental 1 1

CHART mysql_$x.table_locks '' "mysql Tables Locks" "locks/s" locks mysql.table_locks line $((mysql_priority + 4)) $mysql_update_every
DIMENSION Table_locks_immediate immediate incremental 1 1
DIMENSION Table_locks_waited waited incremental -1 1

CHART mysql_$x.join_issues '' "mysql Select Join Issues" "joins/s" issues mysql.join_issues line $((mysql_priority + 5)) $mysql_update_every
DIMENSION Select_full_join full_join incremental 1 1
DIMENSION Select_full_range_join full_range_join incremental 1 1
DIMENSION Select_range range incremental 1 1
DIMENSION Select_range_check range_check incremental 1 1
DIMENSION Select_scan scan incremental 1 1

CHART mysql_$x.sort_issues '' "mysql Sort Issues" "issues/s" issues mysql.sort.issues line $((mysql_priority + 6)) $mysql_update_every
DIMENSION Sort_merge_passes merge_passes incremental 1 1
DIMENSION Sort_range range incremental 1 1
DIMENSION Sort_scan scan incremental 1 1

CHART mysql_$x.tmp '' "mysql Tmp Operations" "counter" temporaries mysql.tmp line $((mysql_priority + 7)) $mysql_update_every
DIMENSION Created_tmp_disk_tables disk_tables incremental 1 1
DIMENSION Created_tmp_files files incremental 1 1
DIMENSION Created_tmp_tables tables incremental 1 1

CHART mysql_$x.connections '' "mysql Connections" "connections/s" connections mysql.connections line $((mysql_priority + 8)) $mysql_update_every
DIMENSION Connections all incremental 1 1
DIMENSION Aborted_connects aborded incremental 1 1

CHART mysql_$x.binlog_cache '' "mysql Binlog Cache" "transactions/s" binlog mysql.binlog_cache line $((mysql_priority + 9)) $mysql_update_every
DIMENSION Binlog_cache_disk_use disk incremental 1 1
DIMENSION Binlog_cache_use all incremental 1 1

CHART mysql_$x.threads '' "mysql Threads" "threads" threads mysql.threads line $((mysql_priority + 10)) $mysql_update_every
DIMENSION Threads_connected connected absolute 1 1
DIMENSION Threads_created created incremental 1 1
DIMENSION Threads_cached cached absolute -1 1
DIMENSION Threads_running running absolute 1 1

CHART mysql_$x.thread_cache_misses '' "mysql Threads Cache Misses" "misses" threads mysql.thread_cache_misses area $((mysql_priority + 11)) $mysql_update_every
DIMENSION misses misses absolute 1 100

CHART mysql_$x.innodb_io '' "mysql InnoDB I/O Bandwidth" "kilobytes/s" innodb mysql.innodb_io area $((mysql_priority + 12)) $mysql_update_every
DIMENSION Innodb_data_read read incremental 1 1024
DIMENSION Innodb_data_written write incremental -1 1024

CHART mysql_$x.innodb_io_ops '' "mysql InnoDB I/O Operations" "operations/s" innodb mysql.innodb_io_ops line $((mysql_priority + 13)) $mysql_update_every
DIMENSION Innodb_data_reads reads incremental 1 1
DIMENSION Innodb_data_writes writes incremental -1 1
DIMENSION Innodb_data_fsyncs fsyncs incremental 1 1

CHART mysql_$x.innodb_io_pending_ops '' "mysql InnoDB Pending I/O Operations" "operations" innodb mysql.innodb_io_pending_ops line $((mysql_priority + 14)) $mysql_update_every
DIMENSION Innodb_data_pending_reads reads absolute 1 1
DIMENSION Innodb_data_pending_writes writes absolute -1 1
DIMENSION Innodb_data_pending_fsyncs fsyncs absolute 1 1

CHART mysql_$x.innodb_log '' "mysql InnoDB Log Operations" "operations/s" innodb mysql.innodb_log line $((mysql_priority + 15)) $mysql_update_every
DIMENSION Innodb_log_waits waits incremental 1 1
DIMENSION Innodb_log_write_requests write_requests incremental -1 1
DIMENSION Innodb_log_writes writes incremental -1 1

CHART mysql_$x.innodb_os_log '' "mysql InnoDB OS Log Operations" "operations" innodb mysql.innodb_os_log line $((mysql_priority + 16)) $mysql_update_every
DIMENSION Innodb_os_log_fsyncs fsyncs incremental 1 1
DIMENSION Innodb_os_log_pending_fsyncs pending_fsyncs absolute 1 1
DIMENSION Innodb_os_log_pending_writes pending_writes absolute -1 1

CHART mysql_$x.innodb_os_log_io '' "mysql InnoDB OS Log Bandwidth" "kilobytes/s" innodb mysql.innodb_os_log_io area $((mysql_priority + 17)) $mysql_update_every
DIMENSION Innodb_os_log_written write incremental -1 1024

CHART mysql_$x.innodb_cur_row_lock '' "mysql InnoDB Current Row Locks" "operations" innodb mysql.innodb_cur_row_lock area $((mysql_priority + 18)) $mysql_update_every
DIMENSION Innodb_row_lock_current_waits current_waits absolute 1 1

CHART mysql_$x.innodb_rows '' "mysql InnoDB Row Operations" "operations/s" innodb mysql.innodb_rows area $((mysql_priority + 19)) $mysql_update_every
DIMENSION Innodb_rows_read read incremental 1 1
DIMENSION Innodb_rows_deleted deleted incremental -1 1
DIMENSION Innodb_rows_inserted inserted incremental 1 1
DIMENSION Innodb_rows_updated updated incremental -1 1

CHART mysql_$x.innodb_buffer_pool_pages '' "mysql InnoDB Buffer Pool Pages" "pages" innodb mysql.innodb_buffer_pool_pages line $((mysql_priority + 20)) $mysql_update_every
DIMENSION Innodb_buffer_pool_pages_data data absolute 1 1
DIMENSION Innodb_buffer_pool_pages_dirty dirty absolute -1 1
DIMENSION Innodb_buffer_pool_pages_free free absolute 1 1
DIMENSION Innodb_buffer_pool_pages_flushed flushed incremental -1 1
DIMENSION Innodb_buffer_pool_pages_misc misc absolute -1 1
DIMENSION Innodb_buffer_pool_pages_total total absolute 1 1

CHART mysql_$x.innodb_buffer_pool_bytes '' "mysql InnoDB Buffer Pool Bytes" "MB" innodb mysql.innodb_buffer_pool_bytes area $((mysql_priority + 21)) $mysql_update_every
DIMENSION Innodb_buffer_pool_bytes_data data absolute 1 $((1024 * 1024))
DIMENSION Innodb_buffer_pool_bytes_dirty dirty absolute -1 $((1024 * 1024))

CHART mysql_$x.innodb_buffer_pool_read_ahead '' "mysql InnoDB Buffer Pool Read Ahead" "operations/s" innodb mysql.innodb_buffer_pool_read_ahead area $((mysql_priority + 22)) $mysql_update_every
DIMENSION Innodb_buffer_pool_read_ahead all incremental 1 1
DIMENSION Innodb_buffer_pool_read_ahead_evicted evicted incremental -1 1
DIMENSION Innodb_buffer_pool_read_ahead_rnd random incremental 1 1

CHART mysql_$x.innodb_buffer_pool_reqs '' "mysql InnoDB Buffer Pool Requests" "requests/s" innodb mysql.innodb_buffer_pool_reqs area $((mysql_priority + 23)) $mysql_update_every
DIMENSION Innodb_buffer_pool_read_requests reads incremental 1 1
DIMENSION Innodb_buffer_pool_write_requests writes incremental -1 1

CHART mysql_$x.innodb_buffer_pool_ops '' "mysql InnoDB Buffer Pool Operations" "operations/s" innodb mysql.innodb_buffer_pool_ops area $((mysql_priority + 24)) $mysql_update_every
DIMENSION Innodb_buffer_pool_reads 'disk reads' incremental 1 1
DIMENSION Innodb_buffer_pool_wait_free 'wait free' incremental -1 1

CHART mysql_$x.qcache_ops '' "mysql QCache Operations" "queries/s" qcache mysql.qcache_ops line $((mysql_priority + 25)) $mysql_update_every
DIMENSION Qcache_hits hits incremental 1 1
DIMENSION Qcache_lowmem_prunes 'lowmem prunes' incremental -1 1
DIMENSION Qcache_inserts inserts incremental 1 1
DIMENSION Qcache_not_cached 'not cached' incremental -1 1

CHART mysql_$x.qcache '' "mysql QCache Queries in Cache" "queries" qcache mysql.qcache line $((mysql_priority + 26)) $mysql_update_every
DIMENSION Qcache_queries_in_cache queries absolute 1 1

CHART mysql_$x.qcache_freemem '' "mysql QCache Free Memory" "MB" qcache mysql.qcache_freemem area $((mysql_priority + 27)) $mysql_update_every
DIMENSION Qcache_free_memory free absolute 1 $((1024 * 1024))

CHART mysql_$x.qcache_memblocks '' "mysql QCache Memory Blocks" "blocks" qcache mysql.qcache_memblocks line $((mysql_priority + 28)) $mysql_update_every
DIMENSION Qcache_free_blocks free absolute 1 1
DIMENSION Qcache_total_blocks total absolute 1 1

CHART mysql_$x.key_blocks '' "mysql MyISAM Key Cache Blocks" "blocks" myisam mysql.key_blocks line $((mysql_priority + 29)) $mysql_update_every
DIMENSION Key_blocks_unused unused absolute 1 1
DIMENSION Key_blocks_used used absolute -1 1
DIMENSION Key_blocks_not_flushed 'not flushed' absolute 1 1

CHART mysql_$x.key_requests '' "mysql MyISAM Key Cache Requests" "requests/s" myisam mysql.key_requests area $((mysql_priority + 30)) $mysql_update_every
DIMENSION Key_read_requests reads incremental 1 1
DIMENSION Key_write_requests writes incremental -1 1

CHART mysql_$x.key_disk_ops '' "mysql MyISAM Key Cache Disk Operations" "operations/s" myisam mysql.key_disk_ops area $((mysql_priority + 31)) $mysql_update_every
DIMENSION Key_reads reads incremental 1 1
DIMENSION Key_writes writes incremental -1 1

CHART mysql_$x.files '' "mysql Open Files" "files" files mysql.files line $((mysql_priority + 32)) $mysql_update_every
DIMENSION Open_files files absolute 1 1

CHART mysql_$x.files_rate '' "mysql Opened Files Rate" "files/s" files mysql.files_rate line $((mysql_priority + 33)) $mysql_update_every
DIMENSION Opened_files files incremental 1 1
EOF

	if [ ! -z "${mysql_data[Binlog_stmt_cache_disk_use]}" ]
		then
		cat <<EOF
CHART mysql_$x.binlog_stmt_cache '' "mysql Binlog Statement Cache" "statements/s" binlog mysql.binlog_stmt_cache line $((mysql_priority + 50)) $mysql_update_every
DIMENSION Binlog_stmt_cache_disk_use disk incremental 1 1
DIMENSION Binlog_stmt_cache_use all incremental 1 1
EOF
	fi

	if [ ! -z "${mysql_data[Connection_errors_accept]}" ]
		then
		cat <<EOF
CHART mysql_$x.connection_errors '' "mysql Connection Errors" "connections/s" connections mysql.connection_errors line $((mysql_priority + 51)) $mysql_update_every
DIMENSION Connection_errors_accept accept incremental 1 1
DIMENSION Connection_errors_internal internal incremental 1 1
DIMENSION Connection_errors_max_connections max incremental 1 1
DIMENSION Connection_errors_peer_addr peer_addr incremental 1 1
DIMENSION Connection_errors_select select incremental 1 1
DIMENSION Connection_errors_tcpwrap tcpwrap incremental 1 1
EOF
	fi

	done
	return 0
}


mysql_update() {
	# the first argument to this function is the microseconds since last update
	# pass this parameter to the BEGIN statement (see bellow).

	# do all the work to collect / calculate the values
	# for each dimension
	# remember: KEEP IT SIMPLE AND SHORT

	local m x
	for m in "${!mysql_ids[@]}"
	do
		x="${mysql_ids[$m]}"
		mysql_get "${mysql_cmds[$m]}" ${mysql_opts[$m]}

		if [ $? -ne 0 ]
			then
			unset mysql_ids[$m]
			unset mysql_opts[$m]
			unset mysql_cmds[$m]
			error "failed to get values for '$m', disabling it."
			continue
		fi

		# write the result of the work.
		cat <<VALUESEOF
BEGIN mysql_$x.net $1
SET Bytes_received = ${mysql_data[Bytes_received]}
SET Bytes_sent = ${mysql_data[Bytes_sent]}
END
BEGIN mysql_$x.queries $1
SET Queries = ${mysql_data[Queries]}
SET Questions = ${mysql_data[Questions]}
SET Slow_queries = ${mysql_data[Slow_queries]}
END
BEGIN mysql_$x.handlers $1
SET Handler_commit = ${mysql_data[Handler_commit]}
SET Handler_delete = ${mysql_data[Handler_delete]}
SET Handler_prepare = ${mysql_data[Handler_prepare]}
SET Handler_read_first = ${mysql_data[Handler_read_first]}
SET Handler_read_key = ${mysql_data[Handler_read_key]}
SET Handler_read_next = ${mysql_data[Handler_read_next]}
SET Handler_read_prev = ${mysql_data[Handler_read_prev]}
SET Handler_read_rnd = ${mysql_data[Handler_read_rnd]}
SET Handler_read_rnd_next = ${mysql_data[Handler_read_rnd_next]}
SET Handler_rollback = ${mysql_data[Handler_rollback]}
SET Handler_savepoint = ${mysql_data[Handler_savepoint]}
SET Handler_savepoint_rollback = ${mysql_data[Handler_savepoint_rollback]}
SET Handler_update = ${mysql_data[Handler_update]}
SET Handler_write = ${mysql_data[Handler_write]}
END
BEGIN mysql_$x.table_locks $1
SET Table_locks_immediate = ${mysql_data[Table_locks_immediate]}
SET Table_locks_waited = ${mysql_data[Table_locks_waited]}
END
BEGIN mysql_$x.join_issues $1
SET Select_full_join = ${mysql_data[Select_full_join]}
SET Select_full_range_join = ${mysql_data[Select_full_range_join]}
SET Select_range = ${mysql_data[Select_range]}
SET Select_range_check = ${mysql_data[Select_range_check]}
SET Select_scan = ${mysql_data[Select_scan]}
END
BEGIN mysql_$x.sort_issues $1
SET Sort_merge_passes = ${mysql_data[Sort_merge_passes]}
SET Sort_range = ${mysql_data[Sort_range]}
SET Sort_scan = ${mysql_data[Sort_scan]}
END
BEGIN mysql_$x.tmp $1
SET Created_tmp_disk_tables = ${mysql_data[Created_tmp_disk_tables]}
SET Created_tmp_files = ${mysql_data[Created_tmp_files]}
SET Created_tmp_tables = ${mysql_data[Created_tmp_tables]}
END
BEGIN mysql_$x.connections $1
SET Connections = ${mysql_data[Connections]}
SET Aborted_connects = ${mysql_data[Aborted_connects]}
END
BEGIN mysql_$x.binlog_cache $1
SET Binlog_cache_disk_use = ${mysql_data[Binlog_cache_disk_use]}
SET Binlog_cache_use = ${mysql_data[Binlog_cache_use]}
END
BEGIN mysql_$x.threads $1
SET Threads_connected = ${mysql_data[Threads_connected]}
SET Threads_created = ${mysql_data[Threads_created]}
SET Threads_cached = ${mysql_data[Threads_cached]}
SET Threads_running = ${mysql_data[Threads_running]}
END
BEGIN mysql_$x.thread_cache_misses $1
SET misses = ${mysql_data[Thread_cache_misses]}
END
BEGIN mysql_$x.innodb_io $1
SET Innodb_data_read = ${mysql_data[Innodb_data_read]}
SET Innodb_data_written = ${mysql_data[Innodb_data_written]}
END
BEGIN mysql_$x.innodb_io_ops $1
SET Innodb_data_reads = ${mysql_data[Innodb_data_reads]}
SET Innodb_data_writes = ${mysql_data[Innodb_data_writes]}
SET Innodb_data_fsyncs = ${mysql_data[Innodb_data_fsyncs]}
END
BEGIN mysql_$x.innodb_io_pending_ops $1
SET Innodb_data_pending_reads = ${mysql_data[Innodb_data_pending_reads]}
SET Innodb_data_pending_writes = ${mysql_data[Innodb_data_pending_writes]}
SET Innodb_data_pending_fsyncs = ${mysql_data[Innodb_data_pending_fsyncs]}
END
BEGIN mysql_$x.innodb_log $1
SET Innodb_log_waits = ${mysql_data[Innodb_log_waits]}
SET Innodb_log_write_requests = ${mysql_data[Innodb_log_write_requests]}
SET Innodb_log_writes = ${mysql_data[Innodb_log_writes]}
END
BEGIN mysql_$x.innodb_os_log $1
SET Innodb_os_log_fsyncs = ${mysql_data[Innodb_os_log_fsyncs]}
SET Innodb_os_log_pending_fsyncs = ${mysql_data[Innodb_os_log_pending_fsyncs]}
SET Innodb_os_log_pending_writes = ${mysql_data[Innodb_os_log_pending_writes]}
END
BEGIN mysql_$x.innodb_os_log_io $1
SET Innodb_os_log_written = ${mysql_data[Innodb_os_log_written]}
END
BEGIN mysql_$x.innodb_cur_row_lock $1
SET Innodb_row_lock_current_waits = ${mysql_data[Innodb_row_lock_current_waits]}
END
BEGIN mysql_$x.innodb_rows $1
SET Innodb_rows_inserted = ${mysql_data[Innodb_rows_inserted]}
SET Innodb_rows_read = ${mysql_data[Innodb_rows_read]}
SET Innodb_rows_updated = ${mysql_data[Innodb_rows_updated]}
SET Innodb_rows_deleted = ${mysql_data[Innodb_rows_deleted]}
END
BEGIN mysql_$x.innodb_buffer_pool_pages $1
SET Innodb_buffer_pool_pages_data = ${mysql_data[Innodb_buffer_pool_pages_data]}
SET Innodb_buffer_pool_pages_dirty = ${mysql_data[Innodb_buffer_pool_pages_dirty]}
SET Innodb_buffer_pool_pages_free = ${mysql_data[Innodb_buffer_pool_pages_free]}
SET Innodb_buffer_pool_pages_flushed = ${mysql_data[Innodb_buffer_pool_pages_flushed]}
SET Innodb_buffer_pool_pages_misc = ${mysql_data[Innodb_buffer_pool_pages_misc]}
SET Innodb_buffer_pool_pages_total = ${mysql_data[Innodb_buffer_pool_pages_total]}
END
BEGIN mysql_$x.innodb_buffer_pool_bytes $1
SET Innodb_buffer_pool_bytes_data = ${mysql_data[Innodb_buffer_pool_bytes_data]}
SET Innodb_buffer_pool_bytes_dirty = ${mysql_data[Innodb_buffer_pool_bytes_dirty]}
END
BEGIN mysql_$x.innodb_buffer_pool_read_ahead $1
SET Innodb_buffer_pool_read_ahead = ${mysql_data[Innodb_buffer_pool_read_ahead]}
SET Innodb_buffer_pool_read_ahead_evicted = ${mysql_data[Innodb_buffer_pool_read_ahead_evicted]}
SET Innodb_buffer_pool_read_ahead_rnd = ${mysql_data[Innodb_buffer_pool_read_ahead_rnd]}
END
BEGIN mysql_$x.innodb_buffer_pool_reqs $1
SET Innodb_buffer_pool_read_requests = ${mysql_data[Innodb_buffer_pool_read_requests]}
SET Innodb_buffer_pool_write_requests = ${mysql_data[Innodb_buffer_pool_write_requests]}
END
BEGIN mysql_$x.innodb_buffer_pool_ops $1
SET Innodb_buffer_pool_reads = ${mysql_data[Innodb_buffer_pool_reads]}
SET Innodb_buffer_pool_wait_free = ${mysql_data[Innodb_buffer_pool_wait_free]}
END
BEGIN mysql_$x.qcache_ops $1
SET Qcache_hits hits = ${mysql_data[Qcache_hits]}
SET Qcache_lowmem_prunes = ${mysql_data[Qcache_lowmem_prunes]}
SET Qcache_inserts = ${mysql_data[Qcache_inserts]}
SET Qcache_not_cached = ${mysql_data[Qcache_not_cached]}
END
BEGIN mysql_$x.qcache $1
SET Qcache_queries_in_cache = ${mysql_data[Qcache_queries_in_cache]}
END
BEGIN mysql_$x.qcache_freemem $1
SET Qcache_free_memory = ${mysql_data[Qcache_free_memory]}
END
BEGIN mysql_$x.qcache_memblocks $1
SET Qcache_free_blocks = ${mysql_data[Qcache_free_blocks]}
SET Qcache_total_blocks = ${mysql_data[Qcache_total_blocks]}
END
BEGIN mysql_$x.key_blocks $1
SET Key_blocks_unused = ${mysql_data[Key_blocks_unused]}
SET Key_blocks_used = ${mysql_data[Key_blocks_used]}
SET Key_blocks_not_flushed = ${mysql_data[Key_blocks_not_flushed]}
END
BEGIN mysql_$x.key_requests $1
SET Key_read_requests = ${mysql_data[Key_read_requests]}
SET Key_write_requests = ${mysql_data[Key_write_requests]}
END
BEGIN mysql_$x.key_disk_ops $1
SET Key_reads = ${mysql_data[Key_reads]}
SET Key_writes = ${mysql_data[Key_writes]}
END
BEGIN mysql_$x.files $1
SET Open_files = ${mysql_data[Open_files]}
END
BEGIN mysql_$x.files_rate $1
SET Opened_files = ${mysql_data[Opened_files]}
END
VALUESEOF

		if [ ! -z "${mysql_data[Binlog_stmt_cache_disk_use]}" ]
			then
			cat <<VALUESEOF
BEGIN mysql_$x.binlog_stmt_cache $1
SET Binlog_stmt_cache_disk_use = ${mysql_data[Binlog_stmt_cache_disk_use]}
SET Binlog_stmt_cache_use = ${mysql_data[Binlog_stmt_cache_use]}
END
VALUESEOF
		fi

		if [ ! -z "${mysql_data[Connection_errors_accept]}" ]
			then
			cat <<VALUESEOF
BEGIN mysql_$x.connection_errors $1
SET Connection_errors_accept = ${mysql_data[Connection_errors_accept]}
SET Connection_errors_internal = ${mysql_data[Connection_errors_internal]}
SET Connection_errors_max_connections = ${mysql_data[Connection_errors_max_connections]}
SET Connection_errors_peer_addr = ${mysql_data[Connection_errors_peer_addr]}
SET Connection_errors_select = ${mysql_data[Connection_errors_select]}
SET Connection_errors_tcpwrap = ${mysql_data[Connection_errors_tcpwrap]}
END
VALUESEOF
		fi
	done

	[ ${#mysql_ids[@]} -eq 0 ] && error "no mysql servers left active." && return 1
	return 0
}

