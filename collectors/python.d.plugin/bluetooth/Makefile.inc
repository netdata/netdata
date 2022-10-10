# SPDX-License-Identifier: GPL-3.0-or-later

# install these files
dist_python_DATA       += bluetooth/bluetooth.chart.py
dist_pythonconfig_DATA += bluetooth/bluetooth.conf

# do not install these files, but include them in the distribution
dist_noinst_DATA       += bluetooth/README.md bluetooth/Makefile.inc
