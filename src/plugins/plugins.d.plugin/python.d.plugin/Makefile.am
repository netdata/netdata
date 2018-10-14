# SPDX-License-Identifier: GPL-3.0-or-later

MAINTAINERCLEANFILES = $(srcdir)/Makefile.in
CLEANFILES = \
	python.d.plugin \
	$(NULL)

include $(top_srcdir)/build/subst.inc
SUFFIXES = .in

dist_libconfig_DATA = \
    python.d.conf \
    $(NULL)

dist_plugins_SCRIPTS = \
	python.d.plugin \
	$(NULL)

dist_noinst_DATA = \
	python.d.plugin.in \
	README.md \
	$(NULL)

pythonconfigdir=$(libconfigdir)/python.d
dist_pythonconfig_DATA = \
    apache/apache.conf \
    beanstalk/beanstalk.conf \
    bind_rndc/bind_rndc.conf \
    boinc/boinc.conf \
    ceph/ceph.conf \
    chrony/chrony.conf \
    couchdb/couchdb.conf \
    cpuidle/cpuidle.conf \
    cpufreq/cpufreq.conf \
    dns_query_time/dns_query_time.conf \
    dnsdist/dnsdist.conf \
    dockerd/dockerd.conf \
    dovecot/dovecot.conf \
    elasticsearch/elasticsearch.conf \
    example/example.conf \
    exim/exim.conf \
    fail2ban/fail2ban.conf \
    freeradius/freeradius.conf \
    go_expvar/go_expvar.conf \
    haproxy/haproxy.conf \
    hddtemp/hddtemp.conf \
    httpcheck/httpcheck.conf \
    icecast/icecast.conf \
    ipfs/ipfs.conf \
    isc_dhcpd/isc_dhcpd.conf \
    linux_power_supply/linux_power_supply.conf \
    litespeed/litespeed.conf \
    logind/logind.conf \
    mdstat/mdstat.conf \
    megacli/megacli.conf \
    memcached/memcached.conf \
    mongodb/mongodb.conf \
    monit/monit.conf \
    mysql/mysql.conf \
    nginx/nginx.conf \
    nginx_plus/nginx_plus.conf \
    nsd/nsd.conf \
    ntpd/ntpd.conf \
    ovpn_status_log/ovpn_status_log.conf \
    phpfpm/phpfpm.conf \
    portcheck/portcheck.conf \
    postfix/postfix.conf \
    postgres/postgres.conf \
    powerdns/powerdns.conf \
    puppet/puppet.conf \
    rabbitmq/rabbitmq.conf \
    redis/redis.conf \
    rethinkdbs/rethinkdbs.conf \
    retroshare/retroshare.conf \
    samba/samba.conf \
    sensors/sensors.conf \
    springboot/springboot.conf \
    spigotmc/spigotmc.conf \
    squid/squid.conf \
    smartd_log/smartd_log.conf \
    tomcat/tomcat.conf \
    traefik/traefik.conf \
    unbound/unbound.conf \
    varnish/varnish.conf \
    w1sensor/w1sensor.conf \
    web_log/web_log.conf \
    $(NULL)

dist_python_SCRIPTS = \
    $(NULL)

dist_python_DATA = \
    apache/apache.chart.py \
    beanstalk/beanstalk.chart.py \
    bind_rndc/bind_rndc.chart.py \
    boinc/boinc.chart.py \
    ceph/ceph.chart.py \
    chrony/chrony.chart.py \
    couchdb/couchdb.chart.py \
    cpufreq/cpufreq.chart.py \
    cpuidle/cpuidle.chart.py \
    dns_query_time/dns_query_time.chart.py \
    dnsdist/dnsdist.chart.py \
    dockerd/dockerd.chart.py \
    dovecot/dovecot.chart.py \
    elasticsearch/elasticsearch.chart.py \
    example/example.chart.py \
    exim/exim.chart.py \
    fail2ban/fail2ban.chart.py \
    freeradius/freeradius.chart.py \
    go_expvar/go_expvar.chart.py \
    haproxy/haproxy.chart.py \
    hddtemp/hddtemp.chart.py \
    httpcheck/httpcheck.chart.py \
    icecast/icecast.chart.py \
    ipfs/ipfs.chart.py \
    isc_dhcpd/isc_dhcpd.chart.py \
    linux_power_supply/linux_power_supply.chart.py \
    litespeed/litespeed.chart.py \
    logind/logind.chart.py \
    mdstat/mdstat.chart.py \
    megacli/megacli.chart.py \
    memcached/memcached.chart.py \
    mongodb/mongodb.chart.py \
    monit/monit.chart.py \
    mysql/mysql.chart.py \
    nginx/nginx.chart.py \
    nginx_plus/nginx_plus.chart.py \
    nsd/nsd.chart.py \
    ntpd/ntpd.chart.py \
    ovpn_status_log/ovpn_status_log.chart.py \
    phpfpm/phpfpm.chart.py \
    portcheck/portcheck.chart.py \
    postfix/postfix.chart.py \
    postgres/postgres.chart.py \
    powerdns/powerdns.chart.py \
    puppet/puppet.chart.py \
    rabbitmq/rabbitmq.chart.py \
    redis/redis.chart.py \
    rethinkdbs/rethinkdbs.chart.py \
    retroshare/retroshare.chart.py \
    samba/samba.chart.py \
    sensors/sensors.chart.py \
    spigotmc/spigotmc.chart.py \
    springboot/springboot.chart.py \
    squid/squid.chart.py \
    smartd_log/smartd_log.chart.py \
    tomcat/tomcat.chart.py \
    traefik/traefik.chart.py \
    unbound/unbound.chart.py \
    varnish/varnish.chart.py \
    w1sensor/w1sensor.chart.py \
    web_log/web_log.chart.py \
    $(NULL)

pythonmodulesdir=$(pythondir)/python_modules
dist_pythonmodules_DATA = \
    python_modules/__init__.py \
    $(NULL)

basesdir=$(pythonmodulesdir)/bases
dist_bases_DATA = \
    python_modules/bases/__init__.py \
    python_modules/bases/charts.py \
    python_modules/bases/collection.py \
    python_modules/bases/loaders.py \
    python_modules/bases/loggers.py \
    $(NULL)

bases_framework_servicesdir=$(basesdir)/FrameworkServices
dist_bases_framework_services_DATA = \
    python_modules/bases/FrameworkServices/__init__.py \
    python_modules/bases/FrameworkServices/ExecutableService.py \
    python_modules/bases/FrameworkServices/LogService.py \
    python_modules/bases/FrameworkServices/MySQLService.py \
    python_modules/bases/FrameworkServices/SimpleService.py \
    python_modules/bases/FrameworkServices/SocketService.py \
    python_modules/bases/FrameworkServices/UrlService.py \
    $(NULL)

third_partydir=$(pythonmodulesdir)/third_party
dist_third_party_DATA = \
    python_modules/third_party/__init__.py \
    python_modules/third_party/ordereddict.py \
    python_modules/third_party/lm_sensors.py \
    python_modules/third_party/mcrcon.py \
    python_modules/third_party/boinc_client.py \
    python_modules/third_party/monotonic.py \
    $(NULL)

pythonyaml2dir=$(pythonmodulesdir)/pyyaml2
dist_pythonyaml2_DATA = \
    python_modules/pyyaml2/__init__.py \
    python_modules/pyyaml2/composer.py \
    python_modules/pyyaml2/constructor.py \
    python_modules/pyyaml2/cyaml.py \
    python_modules/pyyaml2/dumper.py \
    python_modules/pyyaml2/emitter.py \
    python_modules/pyyaml2/error.py \
    python_modules/pyyaml2/events.py \
    python_modules/pyyaml2/loader.py \
    python_modules/pyyaml2/nodes.py \
    python_modules/pyyaml2/parser.py \
    python_modules/pyyaml2/reader.py \
    python_modules/pyyaml2/representer.py \
    python_modules/pyyaml2/resolver.py \
    python_modules/pyyaml2/scanner.py \
    python_modules/pyyaml2/serializer.py \
    python_modules/pyyaml2/tokens.py \
    $(NULL)

pythonyaml3dir=$(pythonmodulesdir)/pyyaml3
dist_pythonyaml3_DATA = \
    python_modules/pyyaml3/__init__.py \
    python_modules/pyyaml3/composer.py \
    python_modules/pyyaml3/constructor.py \
    python_modules/pyyaml3/cyaml.py \
    python_modules/pyyaml3/dumper.py \
    python_modules/pyyaml3/emitter.py \
    python_modules/pyyaml3/error.py \
    python_modules/pyyaml3/events.py \
    python_modules/pyyaml3/loader.py \
    python_modules/pyyaml3/nodes.py \
    python_modules/pyyaml3/parser.py \
    python_modules/pyyaml3/reader.py \
    python_modules/pyyaml3/representer.py \
    python_modules/pyyaml3/resolver.py \
    python_modules/pyyaml3/scanner.py \
    python_modules/pyyaml3/serializer.py \
    python_modules/pyyaml3/tokens.py \
    $(NULL)

python_urllib3dir=$(pythonmodulesdir)/urllib3
dist_python_urllib3_DATA = \
    python_modules/urllib3/__init__.py \
    python_modules/urllib3/_collections.py \
    python_modules/urllib3/connection.py \
    python_modules/urllib3/connectionpool.py \
    python_modules/urllib3/exceptions.py \
    python_modules/urllib3/fields.py \
    python_modules/urllib3/filepost.py \
    python_modules/urllib3/response.py \
    python_modules/urllib3/poolmanager.py \
    python_modules/urllib3/request.py \
    $(NULL)

python_urllib3_utildir=$(python_urllib3dir)/util
dist_python_urllib3_util_DATA = \
    python_modules/urllib3/util/__init__.py \
    python_modules/urllib3/util/connection.py \
    python_modules/urllib3/util/request.py \
    python_modules/urllib3/util/response.py \
    python_modules/urllib3/util/retry.py \
    python_modules/urllib3/util/selectors.py \
    python_modules/urllib3/util/ssl_.py \
    python_modules/urllib3/util/timeout.py \
    python_modules/urllib3/util/url.py \
    python_modules/urllib3/util/wait.py \
    $(NULL)

python_urllib3_packagesdir=$(python_urllib3dir)/packages
dist_python_urllib3_packages_DATA = \
    python_modules/urllib3/packages/__init__.py \
    python_modules/urllib3/packages/ordered_dict.py \
    python_modules/urllib3/packages/six.py \
    $(NULL)

python_urllib3_backportsdir=$(python_urllib3_packagesdir)/backports
dist_python_urllib3_backports_DATA = \
    python_modules/urllib3/packages/backports/__init__.py \
    python_modules/urllib3/packages/backports/makefile.py \
    $(NULL)

python_urllib3_ssl_match_hostnamedir=$(python_urllib3_packagesdir)/ssl_match_hostname
dist_python_urllib3_ssl_match_hostname_DATA = \
    python_modules/urllib3/packages/ssl_match_hostname/__init__.py \
    python_modules/urllib3/packages/ssl_match_hostname/_implementation.py \
    $(NULL)

python_urllib3_contribdir=$(python_urllib3dir)/contrib
dist_python_urllib3_contrib_DATA = \
    python_modules/urllib3/contrib/__init__.py \
    python_modules/urllib3/contrib/appengine.py \
    python_modules/urllib3/contrib/ntlmpool.py \
    python_modules/urllib3/contrib/pyopenssl.py \
    python_modules/urllib3/contrib/securetransport.py \
    python_modules/urllib3/contrib/socks.py \
    $(NULL)

python_urllib3_securetransportdir=$(python_urllib3_contribdir)/_securetransport
dist_python_urllib3_securetransport_DATA = \
    python_modules/urllib3/contrib/_securetransport/__init__.py \
    python_modules/urllib3/contrib/_securetransport/bindings.py \
    python_modules/urllib3/contrib/_securetransport/low_level.py \
    $(NULL)
