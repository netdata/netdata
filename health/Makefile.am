# SPDX-License-Identifier: GPL-3.0-or-later

AUTOMAKE_OPTIONS = subdir-objects
MAINTAINERCLEANFILES = $(srcdir)/Makefile.in

SUBDIRS = \
    notifications \
    $(NULL)

CLEANFILES = \
    $(NULL)

dist_noinst_DATA = \
    README.md \
    $(NULL)

userhealthconfigdir=$(configdir)/health.d
dist_userhealthconfig_DATA = \
    .keep \
    $(NULL)

# Explicitly install directories to avoid permission issues due to umask
install-exec-local:
	$(INSTALL) -d $(DESTDIR)$(userhealthconfigdir)

healthconfigdir=$(libconfigdir)/health.d
dist_healthconfig_DATA = \
    health.d/adaptec_raid.conf \
    health.d/am2320.conf \
    health.d/apache.conf \
    health.d/apcupsd.conf \
    health.d/backend.conf \
    health.d/bcache.conf \
    health.d/beanstalkd.conf \
    health.d/bind_rndc.conf \
    health.d/boinc.conf \
    health.d/btrfs.conf \
    health.d/ceph.conf \
    health.d/cgroups.conf \
    health.d/cpu.conf \
    health.d/couchdb.conf \
    health.d/disks.conf \
    health.d/dnsmasq_dhcp.conf \
    health.d/dockerd.conf \
    health.d/elasticsearch.conf \
    health.d/entropy.conf \
    health.d/fping.conf \
    health.d/ioping.conf \
    health.d/fronius.conf \
    health.d/gearman.conf \
    health.d/haproxy.conf \
    health.d/hdfs.conf \
    health.d/httpcheck.conf \
    health.d/ipc.conf \
    health.d/ipfs.conf \
    health.d/ipmi.conf \
    health.d/isc_dhcpd.conf \
    health.d/kubelet.conf \
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
    health.d/pihole.conf \
    health.d/phpfpm.conf \
    health.d/portcheck.conf \
    health.d/postgres.conf \
    health.d/processes.conf \
    health.d/qos.conf \
    health.d/ram.conf \
    health.d/redis.conf \
    health.d/retroshare.conf \
    health.d/riakkv.conf \
    health.d/scaleio.conf \
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
    health.d/unbound.conf \
    health.d/varnish.conf \
    health.d/vcsa.conf \
    health.d/vsphere.conf \
    health.d/web_log.conf \
    health.d/wmi.conf \
    health.d/x509check.conf \
    health.d/zfs.conf \
    health.d/zookeeper.conf \
    health.d/dbengine.conf \
    $(NULL)
