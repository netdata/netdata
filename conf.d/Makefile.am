#
# Copyright (C) 2015 Alon Bar-Lev <alon.barlev@gmail.com>
#
MAINTAINERCLEANFILES= $(srcdir)/Makefile.in

dist_config_DATA = \
    apps_groups.conf \
    charts.d.conf \
    fping.conf \
    node.d.conf \
    python.d.conf \
    health_alarm_notify.conf \
    health_email_recipients.conf \
    stream.conf \
    $(NULL)

nodeconfigdir=$(configdir)/node.d
dist_nodeconfig_DATA = \
    node.d/README.md \
    node.d/fronius.conf.md \
    node.d/named.conf.md \
    node.d/sma_webbox.conf.md \
    node.d/snmp.conf.md \
    node.d/stiebeleltron.conf.md \
    $(NULL)

pythonconfigdir=$(configdir)/python.d
dist_pythonconfig_DATA = \
    python.d/apache.conf \
    python.d/beanstalk.conf \
    python.d/bind_rndc.conf \
    python.d/chrony.conf \
    python.d/couchdb.conf \
    python.d/cpufreq.conf \
    python.d/dns_query_time.conf \
    python.d/dnsdist.conf \
    python.d/dovecot.conf \
    python.d/elasticsearch.conf \
    python.d/example.conf \
    python.d/exim.conf \
    python.d/fail2ban.conf \
    python.d/freeradius.conf \
    python.d/go_expvar.conf \
    python.d/haproxy.conf \
    python.d/hddtemp.conf \
    python.d/ipfs.conf \
    python.d/isc_dhcpd.conf \
    python.d/mdstat.conf \
    python.d/memcached.conf \
    python.d/mongodb.conf \
    python.d/mysql.conf \
    python.d/nginx.conf \
    python.d/nsd.conf \
    python.d/ovpn_status_log.conf \
    python.d/phpfpm.conf \
    python.d/postfix.conf \
    python.d/postgres.conf \
    python.d/powerdns.conf \
    python.d/rabbitmq.conf \
    python.d/redis.conf \
    python.d/retroshare.conf \
    python.d/samba.conf \
    python.d/sensors.conf \
    python.d/squid.conf \
    python.d/smartd_log.conf \
    python.d/tomcat.conf \
    python.d/varnish.conf \
    python.d/web_log.conf \
    $(NULL)

healthconfigdir=$(configdir)/health.d

dist_healthconfig_DATA = \
    health.d/apache.conf \
    health.d/backend.conf \
    health.d/beanstalkd.conf \
    health.d/bind_rndc.conf \
    health.d/btrfs.conf \
    health.d/cpu.conf \
    health.d/couchdb.conf \
    health.d/disks.conf \
    health.d/elasticsearch.conf \
    health.d/entropy.conf \
    health.d/fping.conf \
    health.d/haproxy.conf \
    health.d/ipc.conf \
    health.d/ipfs.conf \
    health.d/ipmi.conf \
    health.d/isc_dhcpd.conf \
    health.d/lighttpd.conf \
    health.d/mdstat.conf \
    health.d/memcached.conf \
    health.d/memory.conf \
    health.d/mongodb.conf \
    health.d/mysql.conf \
    health.d/named.conf \
    health.d/net.conf \
    health.d/netfilter.conf \
    health.d/nginx.conf \
    health.d/postgres.conf \
    health.d/qos.conf \
    health.d/ram.conf \
    health.d/redis.conf \
    health.d/retroshare.conf \
    health.d/softnet.conf \
    health.d/squid.conf \
    health.d/swap.conf \
    health.d/tcp_conn.conf \
    health.d/tcp_listen.conf \
    health.d/tcp_mem.conf \
    health.d/tcp_orphans.conf \
    health.d/tcp_resets.conf \
    health.d/udp_errors.conf \
    health.d/varnish.conf \
    health.d/web_log.conf \
    health.d/zfs.conf \
    $(NULL)

chartsconfigdir=$(configdir)/charts.d
dist_chartsconfig_DATA = \
    charts.d/apache.conf \
    charts.d/apcupsd.conf \
    charts.d/cpufreq.conf \
    charts.d/exim.conf \
    charts.d/load_average.conf \
    charts.d/mysql.conf \
    charts.d/nut.conf \
    charts.d/phpfpm.conf \
    charts.d/sensors.conf \
    charts.d/tomcat.conf \
    charts.d/ap.conf \
    charts.d/cpu_apps.conf \
    charts.d/example.conf \
    charts.d/hddtemp.conf \
    charts.d/mem_apps.conf \
    charts.d/nginx.conf \
    charts.d/opensips.conf \
    charts.d/postfix.conf \
    charts.d/squid.conf \
    $(NULL)

statsdconfigdir=$(configdir)/statsd.d
dist_statsdconfig_DATA = \
    statsd.d/example.conf \
    $(NULL)
