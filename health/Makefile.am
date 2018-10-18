# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = $(srcdir)/Makefile.in

CLEANFILES = \
	alarm-notify.sh \
	$(NULL)

include $(top_srcdir)/build/subst.inc
SUFFIXES = .in

dist_libconfig_DATA = \
    health_alarm_notify.conf \
    health_email_recipients.conf \
    $(NULL)

dist_plugins_SCRIPTS = \
	alarm-notify.sh \
	alarm-email.sh \
	alarm-test.sh \
	$(NULL)

dist_noinst_DATA = \
	alarm-notify.sh.in \
	README.md \
	$(NULL)

userhealthconfigdir=$(configdir)/health.d
dist_userhealthconfig_DATA = \
    $(top_srcdir)/installer/.keep \
    $(NULL)

healthconfigdir=$(libconfigdir)/health.d
dist_healthconfig_DATA = \
    $(top_srcdir)/installer/.keep \
    health.d/apache.conf \
    health.d/apcupsd.conf \
    health.d/backend.conf \
    health.d/bcache.conf \
    health.d/beanstalkd.conf \
    health.d/bind_rndc.conf \
    health.d/boinc.conf \
    health.d/btrfs.conf \
    health.d/ceph.conf \
    health.d/cpu.conf \
    health.d/couchdb.conf \
    health.d/disks.conf \
    health.d/dockerd.conf \
    health.d/elasticsearch.conf \
    health.d/entropy.conf \
    health.d/fping.conf \
    health.d/fronius.conf \
    health.d/haproxy.conf \
    health.d/httpcheck.conf \
    health.d/ipc.conf \
    health.d/ipfs.conf \
    health.d/ipmi.conf \
    health.d/isc_dhcpd.conf \
    health.d/lighttpd.conf \
    health.d/linux_power_supply.conf \
    health.d/load.conf \
    health.d/mdstat.conf \
    health.d/megacli.conf \
    health.d/memcached.conf \
    health.d/memory.conf \
    health.d/mongodb.conf \
    health.d/mysql.conf \
    health.d/named.conf \
    health.d/net.conf \
    health.d/netfilter.conf \
    health.d/nginx.conf \
    health.d/nginx_plus.conf \
    health.d/portcheck.conf \
    health.d/postgres.conf \
    health.d/qos.conf \
    health.d/ram.conf \
    health.d/redis.conf \
    health.d/retroshare.conf \
    health.d/softnet.conf \
    health.d/squid.conf \
    health.d/stiebeleltron.conf \
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
