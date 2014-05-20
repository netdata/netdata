#!/bin/sh

postfix_postqueue=

postfix_check() {
	# this should return:
	#  - 0 to enable the chart
	#  - 1 to disable the chart

	# try to find the postqueue executable
	if [ -z "$postfix_postqueue" -o ! -x "$postfix_postqueue" ]
	then
		postfix_postqueue="`which postqueue 2>/dev/null`"
		if [ -z "$postfix_postqueue" -o ! -x "$postfix_postqueue" ]
		then
			local d=
			for d in /sbin /usr/sbin /usr/local/sbin
			do
				if [ -x "$d/postqueue" ]
				then
					postfix_postqueue="$d/postqueue"
					break
				fi
			done
		fi
	fi

	if [ -z "$postfix_postqueue" -o ! -x  "$postfix_postqueue" ]
	then
		echo >&2 "Cannot find postqueue. Please set 'postfix_postqueue=/path/to/postqueue' in $confd/postfix.conf"
		return 1
	fi

	return 0
}

postfix_create() {
cat <<EOF
CHART postfix.qemails '' "Postfix Queue Emails" "emails" postfix postfix line 5000 $update_every
DIMENSION emails '' absolute-no-interpolation 1 1
CHART postfix.qsize '' "Postfix Queue Emails Size" "emails size in KB" postfix postfix area 5001 $update_every
DIMENSION size '' absolute 1 1
EOF

	return 0
}

postfix_update() {
	# do all the work to collect / calculate the values
	# for each dimension
	# remember: KEEP IT SIMPLE AND SHORT

	eval `$postfix_postqueue -p | grep "^--" | sed -e "s/-- \([0-9]\+\) Kbytes in \([0-9]\+\) Requests.$/local postfix_q_size=\1\nlocal postfix_q_emails=\2/g" | grep "^local postfix_q_"`
	test -z "$postfix_q_emails" && local postfix_q_emails=0
	test -z "$postfix_q_size" && local postfix_q_size=0

	# write the result of the work.
	cat <<VALUESEOF
BEGIN postfix.qemails $1
SET emails = $postfix_q_emails
END
BEGIN postfix.qsize $1
SET size = $postfix_q_size
END
VALUESEOF

	return 0
}
