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
	example.chart.py \
	exim.chart.py \
	hddtemp.chart.py \
	mysql.chart.py \
	nginx.chart.py \
	phpfpm.chart.py \
	postfix.chart.py \
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
