# SPDX-License-Identifier: GPL-3.0-or-later

# THIS IS NOT A COMPLETE Makefile
# IT IS INCLUDED BY ITS PARENT'S Makefile.am
# IT IS REQUIRED TO REFERENCE ALL FILES RELATIVE TO THE PARENT

# install these files
dist_node_DATA       += bigbluebutton/bigbluebutton.node.js
# dist_nodeconfig_DATA += bigbluebutton/bigbluebutton.conf

# do not install these files, but include them in the distribution
dist_noinst_DATA       += bigbluebutton/README.md bigbluebutton/Makefile.inc

