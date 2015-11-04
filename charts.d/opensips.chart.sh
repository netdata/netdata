#!/bin/sh

opensips_opts="fifo get_statistics all"
opensips_cmd="opensipsctl"
opensips_update_every=5

opensips_get_stats() {
	"$opensips_cmd" $opensips_opts |\
		grep "^\(core\|dialog\|net\|registrar\|shmem\|siptrace\|sl\|tm\|uri\|usrloc\):[a-zA-Z0-9_ -]\+[[:space:]]*=[[:space:]]*[0-9]\+[[:space:]]*$" |\
		sed 	-e "s|-|_|g" \
			-e "s|:|_|g" \
			-e "s|[[:space:]]\+=[[:space:]]\+|=|g" \
			-e "s|[[:space:]]\+$||" \
			-e "s|^[[:space:]]\+||" \
			-e "s|[[:space:]]\+|_|"
}

opensips_check() {
	# check once if the url works
	local x="$(opensips_get_stats | grep "^core_")"
	if [ ! $? -eq 0 -o -z "$x" ]
	then
		echo >&2 "opensips: cannot get global status. Please set opensips_opts='options' whatever needed to get connected to opensips server, in $confd/opensips.conf"
		return 1
	fi

	return 0
}

opensips_create() {
	# create the charts
	cat <<EOF
CHART opensips.dialogs_active '' "OpenSIPS Active Dialogs" "dialogs" opensips '' area 20001 $opensips_update_every
DIMENSION dialog_active_dialogs active absolute 1 1
DIMENSION dialog_early_dialogs early absolute -1 1

CHART opensips.users '' "OpenSIPS Users" "users" opensips '' line 20002 $opensips_update_every
DIMENSION usrloc_registered_users registered absolute 1 1
DIMENSION usrloc_location_users location absolute 1 1
DIMENSION usrloc_location_contacts contacts absolute 1 1
DIMENSION usrloc_location_expires expires incremental -1 $((1 * opensips_update_every))

CHART opensips.registrar '' "OpenSIPS Registrar" "registrations / sec" opensips '' line 20003 $opensips_update_every
DIMENSION registrar_accepted_regs accepted incremental 1 $((1 * opensips_update_every))
DIMENSION registrar_rejected_regs rejected incremental -1 $((1 * opensips_update_every))

CHART opensips.transactions '' "OpenSIPS Transactions" "transactions / sec" opensips '' line 20004 $opensips_update_every
DIMENSION tm_UAS_transactions UAS incremental 1 $((1 * opensips_update_every))
DIMENSION tm_UAC_transactions UAC incremental -1 $((1 * opensips_update_every))

CHART opensips.core_rcv '' "OpenSIPS Core Receives" "queries / sec" opensips '' line 20005 $opensips_update_every
DIMENSION core_rcv_requests requests incremental 1 $((1 * opensips_update_every))
DIMENSION core_rcv_replies replies incremental -1 $((1 * opensips_update_every))

CHART opensips.core_fwd '' "OpenSIPS Core Forwards" "queries / sec" opensips '' line 20006 $opensips_update_every
DIMENSION core_fwd_requests requests incremental 1 $((1 * opensips_update_every))
DIMENSION core_fwd_replies replies incremental -1 $((1 * opensips_update_every))

CHART opensips.core_drop '' "OpenSIPS Core Drops" "queries / sec" opensips '' line 20007 $opensips_update_every
DIMENSION core_drop_requests requests incremental 1 $((1 * opensips_update_every))
DIMENSION core_drop_replies replies incremental -1 $((1 * opensips_update_every))

CHART opensips.core_err '' "OpenSIPS Core Errors" "queries / sec" opensips '' line 20008 $opensips_update_every
DIMENSION core_err_requests requests incremental 1 $((1 * opensips_update_every))
DIMENSION core_err_replies replies incremental -1 $((1 * opensips_update_every))

CHART opensips.core_bad '' "OpenSIPS Core Bad" "queries / sec" opensips '' line 20009 $opensips_update_every
DIMENSION core_bad_URIs_rcvd bad_URIs_rcvd incremental 1 $((1 * opensips_update_every))
DIMENSION core_unsupported_methods unsupported_methods incremental 1 $((1 * opensips_update_every))
DIMENSION core_bad_msg_hdr bad_msg_hdr incremental 1 $((1 * opensips_update_every))

CHART opensips.tm_replies '' "OpenSIPS TM Replies" "replies / sec" opensips '' line 20010 $opensips_update_every
DIMENSION tm_received_replies received incremental 1 $((1 * opensips_update_every))
DIMENSION tm_relayed_replies relayed incremental 1 $((1 * opensips_update_every))
DIMENSION tm_local_replies local incremental 1 $((1 * opensips_update_every))

CHART opensips.transactions_status '' "OpenSIPS Transactions Status" "transactions / sec" opensips '' line 20011 $opensips_update_every
DIMENSION tm_2xx_transactions 2xx incremental 1 $((1 * opensips_update_every))
DIMENSION tm_3xx_transactions 3xx incremental 1 $((1 * opensips_update_every))
DIMENSION tm_4xx_transactions 4xx incremental 1 $((1 * opensips_update_every))
DIMENSION tm_5xx_transactions 5xx incremental 1 $((1 * opensips_update_every))
DIMENSION tm_6xx_transactions 6xx incremental 1 $((1 * opensips_update_every))

CHART opensips.transactions_inuse '' "OpenSIPS InUse Transactions" "transactions" opensips '' line 20012 $opensips_update_every
DIMENSION tm_inuse_transactions inuse absolute 1 1

CHART opensips.sl_replies '' "OpenSIPS SL Replies" "replies / sec" opensips '' line 20013 $opensips_update_every
DIMENSION sl_1xx_replies 1xx incremental 1 $((1 * opensips_update_every))
DIMENSION sl_2xx_replies 2xx incremental 1 $((1 * opensips_update_every))
DIMENSION sl_3xx_replies 3xx incremental 1 $((1 * opensips_update_every))
DIMENSION sl_4xx_replies 4xx incremental 1 $((1 * opensips_update_every))
DIMENSION sl_5xx_replies 5xx incremental 1 $((1 * opensips_update_every))
DIMENSION sl_6xx_replies 6xx incremental 1 $((1 * opensips_update_every))
DIMENSION sl_sent_replies sent incremental 1 $((1 * opensips_update_every))
DIMENSION sl_sent_err_replies error incremental 1 $((1 * opensips_update_every))
DIMENSION sl_received_ACKs ACKed incremental 1 $((1 * opensips_update_every))

CHART opensips.dialogs '' "OpenSIPS Dialogs" "dialogs / sec" opensips '' line 20014 $opensips_update_every
DIMENSION dialog_processed_dialogs processed incremental 1 $((1 * opensips_update_every))
DIMENSION dialog_expired_dialogs expired incremental 1 $((1 * opensips_update_every))
DIMENSION dialog_failed_dialogs failed incremental -1 $((1 * opensips_update_every))

CHART opensips.net_waiting '' "OpenSIPS Network Waiting" "kilobytes" opensips '' line 20015 $opensips_update_every
DIMENSION net_waiting_udp UDP absolute 1 1024
DIMENSION net_waiting_tcp TCP absolute 1 1024

CHART opensips.uri_checks '' "OpenSIPS URI Checks" "checks / sec" opensips '' line 20016 $opensips_update_every
DIMENSION uri_positive_checks positive incremental 1 $((1 * opensips_update_every))
DIMENSION uri_negative_checks negative incremental -1 $((1 * opensips_update_every))

CHART opensips.traces '' "OpenSIPS Traces" "traces / sec" opensips '' line 20017 $opensips_update_every
DIMENSION siptrace_traced_requests requests incremental 1 $((1 * opensips_update_every))
DIMENSION siptrace_traced_replies replies incremental -1 $((1 * opensips_update_every))

CHART opensips.shmem '' "OpenSIPS Shared Memory" "kilobytes" opensips '' line 20018 $opensips_update_every
DIMENSION shmem_total_size total absolute 1 1024
DIMENSION shmem_used_size used absolute 1 1024
DIMENSION shmem_real_used_size real_used absolute 1 1024
DIMENSION shmem_max_used_size max_used absolute 1 1024
DIMENSION shmem_free_size free absolute 1 1024

CHART opensips.shmem_fragments '' "OpenSIPS Shared Memory Fragmentation" "fragments" opensips '' line 20019 $opensips_update_every
DIMENSION shmem_fragments fragments absolute 1 1
EOF
	
	return 0
}


opensips_update() {
	# the first argument to this function is the microseconds since last update
	# pass this parameter to the BEGIN statement (see bellow).

	# do all the work to collect / calculate the values
	# for each dimension
	# remember: KEEP IT SIMPLE AND SHORT

	# 1. get the counters page from opensips
	# 2. sed to remove spaces; replace . with _; remove spaces around =; prepend each line with: local opensips_
	# 3. egrep lines starting with:
	#    local opensips_client_http_ then one or more of these a-z 0-9 _ then = and one of more of 0-9
	#    local opensips_server_all_ then one or more of these a-z 0-9 _ then = and one of more of 0-9
	# 4. then execute this as a script with the eval
	#
	# be very carefull with eval:
	# prepare the script and always grep at the end the lines that are usefull, so that
	# even if something goes wrong, no other code can be executed

	eval "local $(opensips_get_stats)"
	[ $? -ne 0 ] && continue

	# write the result of the work.
	cat <<VALUESEOF

BEGIN opensips.dialogs_active $1
SET dialog_active_dialogs = $dialog_active_dialogs
SET dialog_early_dialogs = $dialog_early_dialogs
END

BEGIN opensips.users $1
SET usrloc_registered_users = $usrloc_registered_users
SET usrloc_location_users = $usrloc_location_users
SET usrloc_location_contacts = $usrloc_location_contacts
SET usrloc_location_expires = $usrloc_location_expires
END

BEGIN opensips.registrar $1
SET registrar_accepted_regs = $registrar_accepted_regs
SET registrar_rejected_regs = $registrar_rejected_regs
END

BEGIN opensips.transactions $1
SET tm_UAS_transactions = $tm_UAS_transactions
SET tm_UAC_transactions = $tm_UAC_transactions
END

BEGIN opensips.core_rcv $1
SET core_rcv_requests = $core_rcv_requests
SET core_rcv_replies = $core_rcv_replies
END

BEGIN opensips.core_fwd $1
SET core_fwd_requests = $core_fwd_requests
SET core_fwd_replies = $core_fwd_replies
END

BEGIN opensips.core_drop $1
SET core_drop_requests = $core_drop_requests
SET core_drop_replies = $core_drop_replies
END

BEGIN opensips.core_err $1
SET core_err_requests = $core_err_requests
SET core_err_replies = $core_err_replies
END

BEGIN opensips.core_bad $1
SET core_bad_URIs_rcvd = $core_bad_URIs_rcvd
SET core_unsupported_methods = $core_unsupported_methods
SET core_bad_msg_hdr = $core_bad_msg_hdr
END

BEGIN opensips.tm_replies $1
SET tm_received_replies = $tm_received_replies
SET tm_relayed_replies = $tm_relayed_replies
SET tm_local_replies = $tm_local_replies
END

BEGIN opensips.transactions_status $1
SET tm_2xx_transactions = $tm_2xx_transactions
SET tm_3xx_transactions = $tm_3xx_transactions
SET tm_4xx_transactions = $tm_4xx_transactions
SET tm_5xx_transactions = $tm_5xx_transactions
SET tm_6xx_transactions = $tm_6xx_transactions
END

BEGIN opensips.transactions_inuse $1
SET tm_inuse_transactions = $tm_inuse_transactions
END

BEGIN opensips.sl_replies $1
SET sl_1xx_replies = $sl_1xx_replies
SET sl_2xx_replies = $sl_2xx_replies
SET sl_3xx_replies = $sl_3xx_replies
SET sl_4xx_replies = $sl_4xx_replies
SET sl_5xx_replies = $sl_5xx_replies
SET sl_6xx_replies = $sl_6xx_replies
SET sl_sent_replies = $sl_sent_replies
SET sl_sent_err_replies = $sl_sent_err_replies
SET sl_received_ACKs = $sl_received_ACKs
END

BEGIN opensips.dialogs $1
SET dialog_processed_dialogs = $dialog_processed_dialogs
SET dialog_expired_dialogs = $dialog_expired_dialogs
SET dialog_failed_dialogs = $dialog_failed_dialogs
END

BEGIN opensips.net_waiting $1
SET net_waiting_udp = $net_waiting_udp
SET net_waiting_tcp = $net_waiting_tcp
END

BEGIN opensips.uri_checks $1
SET uri_positive_checks = $uri_positive_checks
SET uri_negative_checks = $uri_negative_checks
END

BEGIN opensips.traces $1
SET siptrace_traced_requests = $siptrace_traced_requests
SET siptrace_traced_replies = $siptrace_traced_replies
END

BEGIN opensips.shmem $1
SET shmem_total_size = $shmem_total_size
SET shmem_used_size = $shmem_used_size
SET shmem_real_used_size = $shmem_real_used_size
SET shmem_max_used_size = $shmem_max_used_size
SET shmem_free_size = $shmem_free_size
END

BEGIN opensips.shmem_fragments $1
SET shmem_fragments = $shmem_fragments
END
VALUESEOF

	return 0
}

