MAINTAINERCLEANFILES= $(srcdir)/Makefile.in
CLEANFILES = \
    python-modules-installer.sh \
    $(NULL)

include $(top_srcdir)/build/subst.inc

SUFFIXES = .in

dist_python_SCRIPTS = \
    python-modules-installer.sh \
    $(NULL)

dist_python_DATA = \
    README.md \
    apache.chart.py \
    beanstalk.chart.py \
    bind_rndc.chart.py \
    ceph.chart.py \
    chrony.chart.py \
    couchdb.chart.py \
    cpufreq.chart.py \
    cpuidle.chart.py \
    dns_query_time.chart.py \
    dnsdist.chart.py \
    dovecot.chart.py \
    elasticsearch.chart.py \
    example.chart.py \
    exim.chart.py \
    fail2ban.chart.py \
    freeradius.chart.py \
    go_expvar.chart.py \
    haproxy.chart.py \
    hddtemp.chart.py \
    httpcheck.chart.py \
    icecast.chart.py \
    ipfs.chart.py \
    isc_dhcpd.chart.py \
    mdstat.chart.py \
    memcached.chart.py \
    mongodb.chart.py \
    mysql.chart.py \
    nginx.chart.py \
    nginx_plus.chart.py \
    nsd.chart.py \
    ntpd.chart.py \
    ovpn_status_log.chart.py \
    phpfpm.chart.py \
    portcheck.chart.py \
    postfix.chart.py \
    postgres.chart.py \
    powerdns.chart.py \
    puppet.chart.py \
    rabbitmq.chart.py \
    redis.chart.py \
    retroshare.chart.py \
    samba.chart.py \
    sensors.chart.py \
    spigotmc.chart.py \
    springboot.chart.py \
    squid.chart.py \
    smartd_log.chart.py \
    tomcat.chart.py \
    traefik.chart.py \
    unbound.chart.py \
    varnish.chart.py \
    web_log.chart.py \
    $(NULL)

pythonmodulesdir=$(pythondir)/python_modules
dist_pythonmodules_DATA = \
    python_modules/__init__.py \
    python_modules/base.py \
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
