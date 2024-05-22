<!--
title: "Installation methods"
description: "Netdata can be installed as a DEB/RPM package, a static binary, a docker container or from source"
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/methods.md
sidebar_label: "Installation methods"
learn_status: "Published"
learn_rel_path: "Installation/Installation methods"
sidebar_position: 30
-->

# Installation methods

Netdata can be installed:

- [As a DEB/RPM package](/packaging/installer/methods/packages.md)
- [As a static binary](/packaging/makeself/README.md)
- [From a git checkout](/packaging/installer/methods/manual.md)
- [As a docker container](/packaging/docker/README.md)

The [one line installer kickstart.sh](/packaging/installer/methods/kickstart.md)
picks the most appropriate method out of the first three for any system
and is the recommended installation method, if you don't use containers.

`kickstart.sh` can also be used for 
[offline installation](/packaging/installer/methods/offline.md),
suitable for air-gapped systems.
