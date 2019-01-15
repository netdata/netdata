# SPDX-License-Identifier: GPL-3.0-or-later

# THIS IS NOT A COMPLETE Makefile
# IT IS INCLUDED BY ITS PARENT'S Makefile.am
# IT IS REQUIRED TO REFERENCE ALL FILES RELATIVE TO THE PARENT

# install these files
dist_python_DATA       += example_prometheus/example_prometheus.chart.py
dist_pythonconfig_DATA += example_prometheus/example_prometheus.conf

# do not install these files, but include them in the distribution
dist_noinst_DATA       += example_prometheus/README.md example_prometheus/Makefile.inc

