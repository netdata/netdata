# SPDX-License-Identifier: GPL-3.0-or-later

# install these files
dist_python_DATA       += am2320/am2320.chart.py
dist_pythonconfig_DATA += am2320/am2320.conf

# do not install these files, but include them in the distribution
dist_noinst_DATA       += am2320/README.md am2320/Makefile.inc
