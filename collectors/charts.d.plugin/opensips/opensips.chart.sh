# shellcheck shell=bash disable=SC1117,SC2154,SC2086
# no need for shebang - this file is loaded from charts.d.plugin
# SPDX-License-Identifier: GPL-3.0-or-later

# netdata
# real-time performance and health monitoring, done right!
# (C) 2016 Costa Tsaousis <costa@tsaousis.gr>
#

opensips_opts="fifo get_statistics all"
opensips_cmd=
opensips_update_every=5
opensips_timeout=2
opensips_priority=80000

opensips_get_stats() {
	run -t $opensips_timeout "$opensips_cmd" $opensips_opts |\
		grep "^\(core\|dialog\|net\|registrar\|shmem\|siptrace\|sl\|tm\|uri\|usrloc\):[a-zA-Z0-9_-]\+[[:space:]]*[=:]\+[[:space:]]*[0-9]\+[[:space:]]*$" |\
		sed \
			-e "s|[[:space:]]*[=:]\+[[:space:]]*\([0-9]\+\)[[:space:]]*$|=\1|g" \
			-e "s|[[:space:]:-]\+|_|g" \
			-e "s|^|opensips_|g"

	local ret=$?
	[ $ret -ne 0 ] && echo "opensips_command_failed=1"
	return $ret
}

opensips_check() {
	# if the user did not provide an opensips_cmd
	# try to find it in the system
	if [ -z "$opensips_cmd" ]
		then
		require_cmd opensipsctl || return 1
	fi

	# check once if the command works
	local x
	x="$(opensips_get_stats | grep "^opensips_core_")"
	# shellcheck disable=SC2181
	if [ ! $? -eq 0 ] || [ -z "$x" ]
	then
		error "cannot get global status. Please set opensips_opts='options' whatever needed to get connected to opensips server, in $confd/opensips.conf"
		return 1
	fi

	return 0
}

opensips_create() {
	# create the charts
	cat <<EOF
CHART opensips.dialogs_active '' "OpenSIPS Active Dialogs" "dialogs" dialogs '' area $((opensips_priority + 1)) $opensips_update_every
DIMENSION dialog_active_dialogs active absolute 1 1
DIMENSION dialog_early_dialogs early absolute -1 1

CHART opensips.users '' "OpenSIPS Users" "users" users '' line $((opensips_priority + 2)) $opensips_update_every
DIMENSION usrloc_registered_users registered absolute 1 1
DIMENSION usrloc_location_users location absolute 1 1
DIMENSION usrloc_location_contacts contacts absolute 1 1
DIMENSION usrloc_location_expires expires incremental -1 1

CHART opensips.registrar '' "OpenSIPS Registrar" "registrations/s" registrar '' line $((opensips_priority + 3)) $opensips_update_every
DIMENSION registrar_accepted_regs accepted incremental 1 1
DIMENSION registrar_rejected_regs rejected incremental -1 1

CHART opensips.transactions '' "OpenSIPS Transactions" "transactions/s" transactions '' line $((opensips_priority + 4)) $opensips_update_every
DIMENSION tm_UAS_transactions UAS incremental 1 1
DIMENSION tm_UAC_transactions UAC incremental -1 1

CHART opensips.core_rcv '' "OpenSIPS Core Receives" "queries/s" core '' line $((opensips_priority + 5)) $opensips_update_every
DIMENSION core_rcv_requests requests incremental 1 1
DIMENSION core_rcv_replies replies incremental -1 1

CHART opensips.core_fwd '' "OpenSIPS Core Forwards" "queries/s" core '' line $((opensips_priority + 6)) $opensips_update_every
DIMENSION core_fwd_requests requests incremental 1 1
DIMENSION core_fwd_replies replies incremental -1 1

CHART opensips.core_drop '' "OpenSIPS Core Drops" "queries/s" core '' line $((opensips_priority + 7)) $opensips_update_every
DIMENSION core_drop_requests requests incremental 1 1
DIMENSION core_drop_replies replies incremental -1 1

CHART opensips.core_err '' "OpenSIPS Core Errors" "queries/s" core '' line $((opensips_priority + 8)) $opensips_update_every
DIMENSION core_err_requests requests incremental 1 1
DIMENSION core_err_replies replies incremental -1 1

CHART opensips.core_bad '' "OpenSIPS Core Bad" "queries/s" core '' line $((opensips_priority + 9)) $opensips_update_every
DIMENSION core_bad_URIs_rcvd bad_URIs_rcvd incremental 1 1
DIMENSION core_unsupported_methods unsupported_methods incremental 1 1
DIMENSION core_bad_msg_hdr bad_msg_hdr incremental 1 1

CHART opensips.tm_replies '' "OpenSIPS TM Replies" "replies/s" transactions '' line $((opensips_priority + 10)) $opensips_update_every
DIMENSION tm_received_replies received incremental 1 1
DIMENSION tm_relayed_replies relayed incremental 1 1
DIMENSION tm_local_replies local incremental 1 1

CHART opensips.transactions_status '' "OpenSIPS Transactions Status" "transactions/s" transactions '' line $((opensips_priority + 11)) $opensips_update_every
DIMENSION tm_2xx_transactions 2xx incremental 1 1
DIMENSION tm_3xx_transactions 3xx incremental 1 1
DIMENSION tm_4xx_transactions 4xx incremental 1 1
DIMENSION tm_5xx_transactions 5xx incremental 1 1
DIMENSION tm_6xx_transactions 6xx incremental 1 1

CHART opensips.transactions_inuse '' "OpenSIPS InUse Transactions" "transactions" transactions '' line $((opensips_priority + 12)) $opensips_update_every
DIMENSION tm_inuse_transactions inuse absolute 1 1

CHART opensips.sl_replies '' "OpenSIPS SL Replies" "replies/s" core '' line $((opensips_priority + 13)) $opensips_update_every
DIMENSION sl_1xx_replies 1xx incremental 1 1
DIMENSION sl_2xx_replies 2xx incremental 1 1
DIMENSION sl_3xx_replies 3xx incremental 1 1
DIMENSION sl_4xx_replies 4xx incremental 1 1
DIMENSION sl_5xx_replies 5xx incremental 1 1
DIMENSION sl_6xx_replies 6xx incremental 1 1
DIMENSION sl_sent_replies sent incremental 1 1
DIMENSION sl_sent_err_replies error incremental 1 1
DIMENSION sl_received_ACKs ACKed incremental 1 1

CHART opensips.dialogs '' "OpenSIPS Dialogs" "dialogs/s" dialogs '' line $((opensips_priority + 14)) $opensips_update_every
DIMENSION dialog_processed_dialogs processed incremental 1 1
DIMENSION dialog_expired_dialogs expired incremental 1 1
DIMENSION dialog_failed_dialogs failed incremental -1 1

CHART opensips.net_waiting '' "OpenSIPS Network Waiting" "kilobytes" net '' line $((opensips_priority + 15)) $opensips_update_every
DIMENSION net_waiting_udp UDP absolute 1 1024
DIMENSION net_waiting_tcp TCP absolute 1 1024

CHART opensips.uri_checks '' "OpenSIPS URI Checks" "checks / sec" uri '' line $((opensips_priority + 16)) $opensips_update_every
DIMENSION uri_positive_checks positive incremental 1 1
DIMENSION uri_negative_checks negative incremental -1 1

CHART opensips.traces '' "OpenSIPS Traces" "traces / sec" traces '' line $((opensips_priority + 17)) $opensips_update_every
DIMENSION siptrace_traced_requests requests incremental 1 1
DIMENSION siptrace_traced_replies replies incremental -1 1

CHART opensips.shmem '' "OpenSIPS Shared Memory" "kilobytes" mem '' line $((opensips_priority + 18)) $opensips_update_every
DIMENSION shmem_total_size total absolute 1 1024
DIMENSION shmem_used_size used absolute 1 1024
DIMENSION shmem_real_used_size real_used absolute 1 1024
DIMENSION shmem_max_used_size max_used absolute 1 1024
DIMENSION shmem_free_size free absolute 1 1024

CHART opensips.shmem_fragments '' "OpenSIPS Shared Memory Fragmentation" "fragments" mem '' line $((opensips_priority + 19)) $opensips_update_every
DIMENSION shmem_fragments fragments absolute 1 1
EOF

	return 0
}

opensips_update() {
	# the first argument to this function is the microseconds since last update
	# pass this parameter to the BEGIN statement (see bellow).

	# do all the work to collect / calculate the values
	# for each dimension

	# 1. get the counters page from opensips
	# 2. sed to remove spaces; replace . with _; remove spaces around =; prepend each line with: local opensips_
	# 3. egrep lines starting with:
	#    local opensips_client_http_ then one or more of these a-z 0-9 _ then = and one of more of 0-9
	#    local opensips_server_all_ then one or more of these a-z 0-9 _ then = and one of more of 0-9
	# 4. then execute this as a script with the eval
	#    be very carefull with eval:
	#    prepare the script and always grep at the end the lines that are usefull, so that
	#    even if something goes wrong, no other code can be executed

	unset \
		opensips_dialog_active_dialogs \
		opensips_dialog_early_dialogs \
		opensips_usrloc_registered_users \
		opensips_usrloc_location_users \
		opensips_usrloc_location_contacts \
		opensips_usrloc_location_expires \
		opensips_registrar_accepted_regs \
		opensips_registrar_rejected_regs \
		opensips_tm_UAS_transactions \
		opensips_tm_UAC_transactions \
		opensips_core_rcv_requests \
		opensips_core_rcv_replies \
		opensips_core_fwd_requests \
		opensips_core_fwd_replies \
		opensips_core_drop_requests \
		opensips_core_drop_replies \
		opensips_core_err_requests \
		opensips_core_err_replies \
		opensips_core_bad_URIs_rcvd \
		opensips_core_unsupported_methods \
		opensips_core_bad_msg_hdr \
		opensips_tm_received_replies \
		opensips_tm_relayed_replies \
		opensips_tm_local_replies \
		opensips_tm_2xx_transactions \
		opensips_tm_3xx_transactions \
		opensips_tm_4xx_transactions \
		opensips_tm_5xx_transactions \
		opensips_tm_6xx_transactions \
		opensips_tm_inuse_transactions \
		opensips_sl_1xx_replies \
		opensips_sl_2xx_replies \
		opensips_sl_3xx_replies \
		opensips_sl_4xx_replies \
		opensips_sl_5xx_replies \
		opensips_sl_6xx_replies \
		opensips_sl_sent_replies \
		opensips_sl_sent_err_replies \
		opensips_sl_received_ACKs \
		opensips_dialog_processed_dialogs \
		opensips_dialog_expired_dialogs \
		opensips_dialog_failed_dialogs \
		opensips_net_waiting_udp \
		opensips_net_waiting_tcp \
		opensips_uri_positive_checks \
		opensips_uri_negative_checks \
		opensips_siptrace_traced_requests \
		opensips_siptrace_traced_replies \
		opensips_shmem_total_size \
		opensips_shmem_used_size \
		opensips_shmem_real_used_size \
		opensips_shmem_max_used_size \
		opensips_shmem_free_size \
		opensips_shmem_fragments

	opensips_command_failed=0
	eval "local $(opensips_get_stats)"
	# shellcheck disable=SC2181
	[ $? -ne 0 ] && return 1

	[ $opensips_command_failed -eq 1 ] && error "failed to get values, disabling." && return 1

	# write the result of the work.
	cat <<VALUESEOF
BEGIN opensips.dialogs_active $1
SET dialog_active_dialogs = $opensips_dialog_active_dialogs
SET dialog_early_dialogs = $opensips_dialog_early_dialogs
END
BEGIN opensips.users $1
SET usrloc_registered_users = $opensips_usrloc_registered_users
SET usrloc_location_users = $opensips_usrloc_location_users
SET usrloc_location_contacts = $opensips_usrloc_location_contacts
SET usrloc_location_expires = $opensips_usrloc_location_expires
END
BEGIN opensips.registrar $1
SET registrar_accepted_regs = $opensips_registrar_accepted_regs
SET registrar_rejected_regs = $opensips_registrar_rejected_regs
END
BEGIN opensips.transactions $1
SET tm_UAS_transactions = $opensips_tm_UAS_transactions
SET tm_UAC_transactions = $opensips_tm_UAC_transactions
END
BEGIN opensips.core_rcv $1
SET core_rcv_requests = $opensips_core_rcv_requests
SET core_rcv_replies = $opensips_core_rcv_replies
END
BEGIN opensips.core_fwd $1
SET core_fwd_requests = $opensips_core_fwd_requests
SET core_fwd_replies = $opensips_core_fwd_replies
END
BEGIN opensips.core_drop $1
SET core_drop_requests = $opensips_core_drop_requests
SET core_drop_replies = $opensips_core_drop_replies
END
BEGIN opensips.core_err $1
SET core_err_requests = $opensips_core_err_requests
SET core_err_replies = $opensips_core_err_replies
END
BEGIN opensips.core_bad $1
SET core_bad_URIs_rcvd = $opensips_core_bad_URIs_rcvd
SET core_unsupported_methods = $opensips_core_unsupported_methods
SET core_bad_msg_hdr = $opensips_core_bad_msg_hdr
END
BEGIN opensips.tm_replies $1
SET tm_received_replies = $opensips_tm_received_replies
SET tm_relayed_replies = $opensips_tm_relayed_replies
SET tm_local_replies = $opensips_tm_local_replies
END
BEGIN opensips.transactions_status $1
SET tm_2xx_transactions = $opensips_tm_2xx_transactions
SET tm_3xx_transactions = $opensips_tm_3xx_transactions
SET tm_4xx_transactions = $opensips_tm_4xx_transactions
SET tm_5xx_transactions = $opensips_tm_5xx_transactions
SET tm_6xx_transactions = $opensips_tm_6xx_transactions
END
BEGIN opensips.transactions_inuse $1
SET tm_inuse_transactions = $opensips_tm_inuse_transactions
END
BEGIN opensips.sl_replies $1
SET sl_1xx_replies = $opensips_sl_1xx_replies
SET sl_2xx_replies = $opensips_sl_2xx_replies
SET sl_3xx_replies = $opensips_sl_3xx_replies
SET sl_4xx_replies = $opensips_sl_4xx_replies
SET sl_5xx_replies = $opensips_sl_5xx_replies
SET sl_6xx_replies = $opensips_sl_6xx_replies
SET sl_sent_replies = $opensips_sl_sent_replies
SET sl_sent_err_replies = $opensips_sl_sent_err_replies
SET sl_received_ACKs = $opensips_sl_received_ACKs
END
BEGIN opensips.dialogs $1
SET dialog_processed_dialogs = $opensips_dialog_processed_dialogs
SET dialog_expired_dialogs = $opensips_dialog_expired_dialogs
SET dialog_failed_dialogs = $opensips_dialog_failed_dialogs
END
BEGIN opensips.net_waiting $1
SET net_waiting_udp = $opensips_net_waiting_udp
SET net_waiting_tcp = $opensips_net_waiting_tcp
END
BEGIN opensips.uri_checks $1
SET uri_positive_checks = $opensips_uri_positive_checks
SET uri_negative_checks = $opensips_uri_negative_checks
END
BEGIN opensips.traces $1
SET siptrace_traced_requests = $opensips_siptrace_traced_requests
SET siptrace_traced_replies = $opensips_siptrace_traced_replies
END
BEGIN opensips.shmem $1
SET shmem_total_size = $opensips_shmem_total_size
SET shmem_used_size = $opensips_shmem_used_size
SET shmem_real_used_size = $opensips_shmem_real_used_size
SET shmem_max_used_size = $opensips_shmem_max_used_size
SET shmem_free_size = $opensips_shmem_free_size
END
BEGIN opensips.shmem_fragments $1
SET shmem_fragments = $opensips_shmem_fragments
END
VALUESEOF

	return 0
}
