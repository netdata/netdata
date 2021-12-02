# SPDX-License-Identifier: GPL-3.0-or-later

# THIS IS NOT A COMPLETE Makefile
# IT IS INCLUDED BY ITS PARENT'S Makefile.am
# IT IS REQUIRED TO REFERENCE ALL FILES RELATIVE TO THE PARENT

# install these files
dist_charts_DATA       += sensors/sensors.chart.sh
dist_chartsconfig_DATA += sensors/sensors.conf

# do not install these files, but include them in the distribution
dist_noinst_DATA       += sensors/README.md sensors/Makefile.inc

