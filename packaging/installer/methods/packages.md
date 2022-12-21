<!--
title: "Install Netdata using native DEB/RPM packages."
description: "Instructions for how to install Netdata using native DEB or RPM packages."
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/packages.md
-->

# Installing Netdata using native DEB or RPM packages.

For most common Linux distributions that use either DEB or RPM packages, Netdata provides pre-built native packages
for current releases in-line with our [official platform support policy](/packaging/PLATFORM_SUPPORT.md). These
packages will be used by default when attempting to install on a supported platform using our [kickstart.sh
installer script](/packaging/installer/methods/kickstart.md).

When using the kickstart script, you can force usage of native DEB or RPM packages by passing the option
`--native-only` when invoking the script. This will cause it to only attempt to use native packages for the install,
and fail if it cannot do so.

## Manual setup of RPM packages.

Netdata’s official RPM repositories are hosted at https://repo.netdata.cloud/repos. We provide four groups of
repositories at that top level:

- `stable`: Contains packages for stable releases of the Netdata Agent.
- `edge`: Contains packages for nightly builds of the Netdata Agent.
- `repoconfig`: Provides packages that set up configuration files for using the other repositories.
- `devel`: Is used for one-off development builds of the Netdata Agent, and can simply be ignored by users.

Within each top level group of repositories, there are directories for each supported group of distributions:

- `el`: Is for Red Hat Enterprise Linux and binary compatible distros, such as CentOS, Alma Linux, and Rocky Linux.
- `fedora`: Is for Fedora and binary compatible distros.
- `ol`: Is for Oracle Linux and binary compatible distros.
- `opensuse`: Is for openSUSE and binary compatible distros.

Under each of those directories is a directory for each supported release of that distribution, and under that a
directory for each supported CPU architecture which contains the actual repository.

For example, for stable release packages for RHEL 9 on 64-bit x86, the full URL for the repository would be
https://repo.netdata.cloud/repos/stable/el/9/x86\_64/

Our RPM packages and repository metadata are signed using a GPG key with a user name of ‘Netdatabot’. The
current key fingerprint is `6588FDD7B14721FE7C3115E6F9177B5265F56346`

If you are explicitly configuring a system to use our repositories, the recommended setup is to download the
appropriate repository configuration package from https://repo.netdata.cloud/repos/repoconfig and install it
directly on the target system using the system package manager. This will ensure any packages needed to use the
repository are also installed, and will help enable a seamless transition if we ever need to change our infrastructure.
