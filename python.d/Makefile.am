MAINTAINERCLEANFILES= $(srcdir)/Makefile.in
CLEANFILES = \
    python-modules-installer.sh \
    $(NULL)

include $(top_srcdir)/build/subst.inc

SUFFIXES = .in

dist_python_SCRIPTS = \
    apache.chart.py \
    apache_cache.chart.py \
    cpufreq.chart.py \
    dovecot.chart.py \
    example.chart.py \
    exim.chart.py \
    hddtemp.chart.py \
    ipfs.chart.py \
    memcached.chart.py \
    mysql.chart.py \
    nginx.chart.py \
    nginx_log.chart.py \
    phpfpm.chart.py \
    postfix.chart.py \
    postgres.chart.py \
    redis.chart.py \
    retroshare.chart.py \
    sensors.chart.py \
    squid.chart.py \
    tomcat.chart.py \
    python-modules-installer.sh \
    $(NULL)

dist_python_DATA = \
    README.md \
    $(NULL)

pythonmodulesdir=$(pythondir)/python_modules
dist_pythonmodules_DATA = \
    python_modules/__init__.py \
    python_modules/base.py \
    python_modules/msg.py \
    python_modules/lm_sensors.py \
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
