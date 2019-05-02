# SPDX-License-Identifier: GPL-3.0-or-later

# THIS IS NOT A COMPLETE Makefile
# IT IS INCLUDED BY ITS PARENT'S Makefile.am
# IT IS REQUIRED TO REFERENCE ALL FILES RELATIVE TO THE PARENT

# install these files
dist_python_DATA       += energid/energid.chart.py
dist_pythonconfig_DATA += energid/energid.conf

# do not install these files, but include them in the distribution
dist_noinst_DATA       += energid/README.md energid/Makefile.inc

