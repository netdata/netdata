MAINTAINERCLEANFILES= $(srcdir)/Makefile.in

include $(top_srcdir)/build/subst.inc

SUFFIXES = .in

dist_python_SCRIPTS = \
	example.chart.py \
	mysql.chart.py \
	phpfpm.chart.py \
	apache.chart.py \
	nginx.chart.py \
	apache_cache.chart.py \
	hddtemp.chart.py \
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
	$(NULL)

