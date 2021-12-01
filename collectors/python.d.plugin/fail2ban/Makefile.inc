# SPDX-License-Identifier: GPL-3.0-or-later

# THIS IS NOT A COMPLETE Makefile
# IT IS INCLUDED BY ITS PARENT'S Makefile.am
# IT IS REQUIRED TO REFERENCE ALL FILES RELATIVE TO THE PARENT

# install these files
dist_python_DATA       += fail2ban/fail2ban.chart.py
dist_pythonconfig_DATA += fail2ban/fail2ban.conf

# do not install these files, but include them in the distribution
dist_noinst_DATA       += fail2ban/README.md fail2ban/Makefile.inc

