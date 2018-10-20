# SPDX-License-Identifier: GPL-3.0-or-later

# THIS IS NOT A COMPLETE Makefile
# IT IS INCLUDED BY ITS PARENT'S Makefile.am
# IT IS REQUIRED TO REFERENCE ALL FILES RELATIVE TO THE PARENT

# install these files
dist_charts_DATA       += load_average/load_average.chart.sh
dist_chartsconfig_DATA += load_average/load_average.conf

# do not install these files, but include them in the distribution
dist_noinst_DATA       += load_average/README.md load_average/Makefile.inc

