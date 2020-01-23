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

dist_python_SCRIPTS = \
    $(NULL)

dist_python_DATA = \
    $(NULL)

userpythonconfigdir=$(configdir)/python.d
dist_userpythonconfig_DATA = \
    $(NULL)

# Explicitly install directories to avoid permission issues due to umask
install-exec-local:
	$(INSTALL) -d $(DESTDIR)$(userpythonconfigdir)

pythonconfigdir=$(libconfigdir)/python.d
dist_pythonconfig_DATA = \
    $(NULL)

include adaptec_raid/Makefile.inc
include am2320/Makefile.inc
include apache/Makefile.inc
include beanstalk/Makefile.inc
include bind_rndc/Makefile.inc
include boinc/Makefile.inc
include ceph/Makefile.inc
include chrony/Makefile.inc
include couchdb/Makefile.inc
include dnsdist/Makefile.inc
include dns_query_time/Makefile.inc
include dockerd/Makefile.inc
include dovecot/Makefile.inc
include elasticsearch/Makefile.inc
include energid/Makefile.inc
include example/Makefile.inc
include exim/Makefile.inc
include fail2ban/Makefile.inc
include freeradius/Makefile.inc
include gearman/Makefile.inc
include go_expvar/Makefile.inc
include haproxy/Makefile.inc
include hddtemp/Makefile.inc
include httpcheck/Makefile.inc
include hpssa/Makefile.inc
include icecast/Makefile.inc
include ipfs/Makefile.inc
include isc_dhcpd/Makefile.inc
include litespeed/Makefile.inc
include logind/Makefile.inc
include megacli/Makefile.inc
include memcached/Makefile.inc
include mongodb/Makefile.inc
include monit/Makefile.inc
include mysql/Makefile.inc
include nginx/Makefile.inc
include nginx_plus/Makefile.inc
include nvidia_smi/Makefile.inc
include nsd/Makefile.inc
include ntpd/Makefile.inc
include ovpn_status_log/Makefile.inc
include openldap/Makefile.inc
include oracledb/Makefile.inc
include phpfpm/Makefile.inc
include portcheck/Makefile.inc
include postfix/Makefile.inc
include postgres/Makefile.inc
include powerdns/Makefile.inc
include proxysql/Makefile.inc
include puppet/Makefile.inc
include rabbitmq/Makefile.inc
include redis/Makefile.inc
include rethinkdbs/Makefile.inc
include retroshare/Makefile.inc
include riakkv/Makefile.inc
include samba/Makefile.inc
include sensors/Makefile.inc
include smartd_log/Makefile.inc
include spigotmc/Makefile.inc
include springboot/Makefile.inc
include squid/Makefile.inc
include tomcat/Makefile.inc
include tor/Makefile.inc
include traefik/Makefile.inc
include unbound/Makefile.inc
include uwsgi/Makefile.inc
include varnish/Makefile.inc
include w1sensor/Makefile.inc
include web_log/Makefile.inc

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
