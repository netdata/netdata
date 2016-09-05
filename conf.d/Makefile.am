#
# Copyright (C) 2015 Alon Bar-Lev <alon.barlev@gmail.com>
#
MAINTAINERCLEANFILES= $(srcdir)/Makefile.in

dist_config_DATA = \
	apps_groups.conf \
	charts.d.conf \
	python.d.conf \
	health_email_recipients.conf \
	$(NULL)

chartsconfigdir=$(configdir)/charts.d
dist_chartsconfig_DATA = \
	$(NULL)

nodeconfigdir=$(configdir)/node.d
dist_nodeconfig_DATA = \
	$(NULL)

pythonconfigdir=$(configdir)/python.d
dist_pythonconfig_DATA = \
	python.d/apache.conf \
	python.d/apache_cache.conf \
	python.d/cpufreq.conf \
	python.d/dovecot.conf \
	python.d/example.conf \
	python.d/exim.conf \
	python.d/hddtemp.conf \
	python.d/ipfs.conf \
	python.d/memcached.conf \
	python.d/mysql.conf \
	python.d/nginx.conf \
	python.d/nginx_log.conf \
	python.d/phpfpm.conf \
	python.d/postfix.conf \
	python.d/redis.conf \
	python.d/sensors.conf \
	python.d/squid.conf \
	python.d/tomcat.conf \
	$(NULL)

healthconfigdir=$(configdir)/health.d
dist_healthconfig_DATA = \
	health.d/apache.conf \
	health.d/cpu.conf \
	health.d/disks.conf \
	health.d/entropy.conf \
	health.d/memcached.conf \
	health.d/mysql.conf \
	health.d/named.conf \
	health.d/net.conf \
	health.d/nginx.conf \
	health.d/qos.conf \
	health.d/ram.conf \
	health.d/redis.conf \
	health.d/swap.conf \
	health.d/squid.conf \
	$(NULL)
