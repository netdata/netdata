# shellcheck shell=bash
# no need for shebang - this file is loaded from charts.d.plugin
# SPDX-License-Identifier: GPL-3.0+

# netdata
# real-time performance and health monitoring, done right!
# (C) 2016 Costa Tsaousis <costa@tsaousis.gr>
#
# Contributed by @jsveiga with PR #480

# the exim command to run
exim_command=

# how frequently to collect queue size
exim_update_every=5

exim_priority=60000

exim_check() {
    if [ -z "${exim_command}" ]
    then
        require_cmd exim || return 1
        exim_command="${EXIM_CMD}"
    fi

	if [ "$(${exim_command} -bpc 2>&1 | grep -c denied)" -ne 0 ]
	then
		error "permission denied - please set 'queue_list_requires_admin = false' in your exim options file"
		return 1
	fi

	return 0
}

exim_create() {
    cat <<EOF
CHART exim_local.qemails '' "Exim Queue Emails" "emails" queue exim.queued.emails line $((exim_priority + 1)) $exim_update_every
DIMENSION emails '' absolute 1 1
EOF
    return 0
}

exim_update() {
    echo "BEGIN exim_local.qemails $1"
    echo "SET emails = $(run "${exim_command}" -bpc)"
    echo "END"
    return 0
}
