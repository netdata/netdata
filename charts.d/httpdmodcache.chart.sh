#!/bin/sh

# Adds graph with percentage count of traffic served through mod_cache. Will only
# sample the last 10 000 log lines every `httpdmodcache_update_every`.
#
# In your configuration file, set `httpdmodcache_logfile` to a logfile containing
# cache-status information from Apache. Make sure this file is readable by netdata.
# See https://httpd.apache.org/docs/current/mod/mod_cache.html#status for details.

httpdmodcache_logfile=""
httpdmodcache_update_every=5
httpdmodcache_priority=60000

httpdmodcache_get_hitcount() {
    tail --lines=10000 "$httpdmodcache_logfile" | grep --count "cache hit"  | tr -d "\n"
}

httpdmodcache_get_misscount() {
    tail --lines=10000 "$httpdmodcache_logfile" | grep --count "cache miss" | tr -d "\n"
}

httpdmodcache_check() {
	if [ ! -f "$httpdmodcache_logfile" ]
	then
		return 1
	fi
	return 0
}

httpdmodcache_create() {
	# create the charts
	cat <<EOF
CHART httpdmodcache.cache '' "httpd cached responses" "percent cached" cached httpdmodcache.cache stacked $((httpdmodcache_priority + 1)) $((httpdmodcache_update_every))
DIMENSION hit 'cache' percentage-of-absolute-row 1 1
DIMENSION miss '' percentage-of-absolute-row 1 1
EOF

	return 0
}

httpdmodcache_update() {
    LOCALE=C
    local httpdmodcache_hitcount=$(httpdmodcache_get_hitcount)
    local httpdmodcache_misscount=$(httpdmodcache_get_misscount)

    # Variables only contains counts, so no need to escape them.
    local httpdmodcache_total=$(echo "$httpdmodcache_hitcount + $httpdmodcache_misscount" | bc)
    local httpdmodcache_hitperc=$(echo "scale=4; ($httpdmodcache_hitcount / $httpdmodcache_total) * 100" | bc | awk '{printf "%.2f", $0}')
    local httpdmodcache_missperc=$(echo "scale=4; ($httpdmodcache_misscount / $httpdmodcache_total) * 100" | bc | awk '{printf "%.2f", $0}')

    cat <<VALUESEOF
BEGIN httpdmodcache.cache $1
SET hit = $httpdmodcache_hitperc
SET miss = $httpdmodcache_missperc
END
VALUESEOF

    return 0
}
