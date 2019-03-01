#!/usr/bin/env bats

validate_metrics() {
	fname="${1}"
	params="${2}"

	if [ ! -f .gitignore ];	then
		echo "Need to run as ./tests/backends/$(basename "$0") from top level directory of git repository"
		return 1
	fi
	curl -sS "http://localhost:19999/api/v1/allmetrics?format=prometheus&prefix=nd&timestamps=no${params}" |
	grep -E 'nd_system_|nd_netdata_|nd_cpu_|nd_services_|nd_system_|nd_net_|nd_disk_|nd_ip_|nd_ipv4_|nd_ipv6_|nd_mem_' |
	sed -ne 's/{.*//p' | sort | uniq > tests/backends/new-${fname}
	diff tests/backends/${fname} tests/backends/new-${fname}
	ret=$?
	rm tests/backends/new-${fname}
}


@test "prometheus raw" {
	validate_metrics prometheus-raw.txt "&data=raw"
}

@test "prometheus avg" {
	validate_metrics prometheus-avg.txt ""
}

@test "prometheus avg oldunits" {
	validate_metrics prometheus-avg-oldunits.txt "&oldunits=yes"
}
