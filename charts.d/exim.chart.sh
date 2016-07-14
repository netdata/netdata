# no need for shebang - this file is loaded from charts.d.plugin

exim_command=

# how frequently to collect queue size
exim_update_every=5

exim_priority=60000

exim_check() {
	if [ -z "$exim_command" -o ! -x "$exim_command" ]
		then
		local d=
		for d in /sbin /usr/sbin /usr/local/sbin
		do
			if [ -x "$d/exim" ]
			then
				exim_command="$d/exim"
				break
			fi
		done
	fi

	if [ -z "$exim_command" -o ! -x  "$exim_command" ]
	then
		echo >&2 "$PROGRAM_NAME: exim: cannot find exim executable. Please set 'exim_command=/path/to/exim' in $confd/exim.conf"
		return 1
	fi

	if [ `$exim_command -bpc 2>&1 | grep -c denied` -ne 0 ]
	then
		echo >&2 "$PROGRAM_NAME: exim: permission denied. Please set 'queue_list_requires_admin = false' in your exim options file"
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
echo "SET emails = " `$exim_command -bpc`
echo "END"
return 0
}
