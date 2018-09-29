# shellcheck shell=bash disable=SC1117
# no need for shebang - this file is loaded from charts.d.plugin
# SPDX-License-Identifier: GPL-3.0+

# netdata
# real-time performance and health monitoring, done right!
# (C) 2016 Costa Tsaousis <costa@tsaousis.gr>
#

# the postqueue command
# if empty, it will use the one found in the system path
postfix_postqueue=

# how frequently to collect queue size
postfix_update_every=15

postfix_priority=60000

postfix_check() {
	# this should return:
	#  - 0 to enable the chart
	#  - 1 to disable the chart

	# try to find the postqueue executable
	if [ -z "$postfix_postqueue" ] || [ ! -x "$postfix_postqueue" ]
	then
		# shellcheck disable=SC2230
		postfix_postqueue="$(which postqueue 2>/dev/null || command -v postqueue 2>/dev/null)"
	fi

	if [ -z "$postfix_postqueue" ] || [ ! -x  "$postfix_postqueue" ]
	then
		# shellcheck disable=SC2154
		error "cannot find postqueue. Please set 'postfix_postqueue=/path/to/postqueue' in $confd/postfix.conf"
		return 1
	fi

	return 0
}

postfix_create() {
cat <<EOF
CHART postfix_local.qemails '' "Postfix Queue Emails" "emails" queue postfix.queued.emails line $((postfix_priority + 1)) $postfix_update_every
DIMENSION emails '' absolute 1 1
CHART postfix_local.qsize '' "Postfix Queue Emails Size" "emails size in KB" queue postfix.queued.size area $((postfix_priority + 2)) $postfix_update_every
DIMENSION size '' absolute 1 1
EOF

	return 0
}

postfix_update() {
	# the first argument to this function is the microseconds since last update
	# pass this parameter to the BEGIN statement (see bellow).

	# do all the work to collect / calculate the values
	# for each dimension
	# remember: KEEP IT SIMPLE AND SHORT

	# 1. execute postqueue -p
	# 2. get the line that begins with --
	# 3. match the 2 numbers on the line and output 2 lines like these:
	#    local postfix_q_size=NUMBER
	#    local postfix_q_emails=NUMBER
	# 4. then execute this a script with the eval
	#
	# be very carefull with eval:
	# prepare the script and always egrep at the end the lines that are usefull, so that
	# even if something goes wrong, no other code can be executed
	postfix_q_emails=0
	postfix_q_size=0

	eval "$(run "$postfix_postqueue" -p |\
		grep "^--" |\
		sed -e "s/-- \([0-9]\+\) Kbytes in \([0-9]\+\) Requests.$/local postfix_q_size=\1\nlocal postfix_q_emails=\2/g" |\
		grep -E "^local postfix_q_(emails|size)=[0-9]+$")"

	# write the result of the work.
	cat <<VALUESEOF
BEGIN postfix_local.qemails $1
SET emails = $postfix_q_emails
END
BEGIN postfix_local.qsize $1
SET size = $postfix_q_size
END
VALUESEOF

	return 0
}
