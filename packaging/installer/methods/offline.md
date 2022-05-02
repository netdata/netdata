<!--
title: "Install Netdata on offline systems"
description: "Install the Netdata Agent on offline/air gapped systems to benefit from real-time, per-second monitoring without connecting to the internet."
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/offline.md
-->

# Install Netdata on offline systems

Curretly, we provide only limited support for installation on systems which do not have a usable internet connection.

Prior to the release of our rewritten kickstart script in early 2022, the `kickstart.sh` and `kickstart-static64.sh`
scripts provided support for installing on such systems. However, this code had not actually functioned correctly
for quite some time, and was thus removed with the rewrite of the kickstart scripts. We are in the process of
implementing support for such installations in the new kickstart code, but as of right now have no estimate of
when this may be ready.

Users of Debian or Ubuntu systems which we support may be able to utilize the `apt-offline` tool provided in the
package repositories for those platforms to install our native packages on such systems, but we do not currently
provide any official support for this use case.
