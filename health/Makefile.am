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
    $(NULL)

# Explicitly install directories to avoid permission issues due to umask
install-exec-local:
	$(INSTALL) -d $(DESTDIR)$(userhealthconfigdir)

healthconfigdir=$(libconfigdir)/health.d
dist_healthconfig_DATA = \
    health.d/adaptec_raid.conf \
    health.d/anomalies.conf \
    health.d/apcupsd.conf \
    health.d/bcache.conf \
    health.d/beanstalkd.conf \
    health.d/bind_rndc.conf \
    health.d/boinc.conf \
    health.d/btrfs.conf \
    health.d/ceph.conf \
    health.d/cgroups.conf \
    health.d/cpu.conf \
    health.d/cockroachdb.conf \
    health.d/consul.conf \
    health.d/disks.conf \
    health.d/dnsmasq_dhcp.conf \
    health.d/dns_query.conf \
    health.d/docker.conf \
    health.d/elasticsearch.conf \
    health.d/entropy.conf \
    health.d/exporting.conf \
    health.d/geth.conf \
    health.d/ioping.conf \
    health.d/gearman.conf \
    health.d/go.d.plugin.conf \
    health.d/haproxy.conf \
    health.d/hdfs.conf \
    health.d/httpcheck.conf \
    health.d/ipc.conf \
    health.d/ipfs.conf \
    health.d/ipmi.conf \
    health.d/isc_dhcpd.conf \
    health.d/kubelet.conf \
    health.d/linux_power_supply.conf \
    health.d/load.conf \
    health.d/mdstat.conf \
    health.d/megacli.conf \
    health.d/memcached.conf \
    health.d/memory.conf \
    health.d/ml.conf \
    health.d/mysql.conf \
    health.d/net.conf \
    health.d/netfilter.conf \
    health.d/nvme.conf \
    health.d/nut.conf \
    health.d/pihole.conf \
    health.d/plugin.conf \
    health.d/ping.conf \
    health.d/postgres.conf \
    health.d/portcheck.conf \
    health.d/processes.conf \
    health.d/python.d.plugin.conf \
    health.d/qos.conf \
    health.d/ram.conf \
    health.d/redis.conf \
    health.d/retroshare.conf \
    health.d/riakkv.conf \
    health.d/scaleio.conf \
    health.d/softnet.conf \
    health.d/synchronization.conf \
    health.d/swap.conf \
    health.d/systemdunits.conf \
    health.d/timex.conf \
    health.d/tcp_conn.conf \
    health.d/tcp_listen.conf \
    health.d/tcp_mem.conf \
    health.d/tcp_orphans.conf \
    health.d/tcp_resets.conf \
    health.d/udp_errors.conf \
    health.d/unbound.conf \
    health.d/vcsa.conf \
    health.d/vernemq.conf \
    health.d/vsphere.conf \
    health.d/web_log.conf \
    health.d/whoisquery.conf \
    health.d/windows.conf \
    health.d/x509check.conf \
    health.d/zfs.conf \
    health.d/dbengine.conf \
    $(NULL)
