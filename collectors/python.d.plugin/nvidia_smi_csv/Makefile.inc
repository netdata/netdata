# SPDX-License-Identifier: GPL-3.0-or-later

# THIS IS NOT A COMPLETE Makefile
# IT IS INCLUDED BY ITS PARENT'S Makefile.am
# IT IS REQUIRED TO REFERENCE ALL FILES RELATIVE TO THE PARENT

# install these files
dist_python_DATA       += nvidia_smi_csv/nvidia_smi_csv.chart.py
dist_pythonconfig_DATA += nvidia_smi_csv/nvidia_smi_csv.conf

# do not install these files, but include them in the distribution
dist_noinst_DATA       += nvidia_smi_csv/README.md nvidia_smi_csv/Makefile.inc
