# Install Netdata using native DEB/RPM packages

For most common Linux distributions that use either DEB or RPM packages, Netdata provides pre-built native packages
for current releases in-line with
our [official platform support policy](/docs/netdata-agent/versions-and-platforms.md).
These packages will be used by default when attempting to install on a supported platform using our
[kickstart.sh installer script](/packaging/installer/methods/kickstart.md).

When using the kickstart script, you can force usage of native DEB or RPM packages by passing the option
`--native-only` when invoking the script. This will cause it to only attempt to use native packages for the install,
and fail if it cannot do so.

> **Note**
>
> In July 2022, we switched hosting of our native packages from Package Cloud to self-hosted repositories.
> Until late 2024 we continued to provide packages via Package Cloud, but we have since then switched to only
> providing packages via our repositories.

## Manual setup of RPM packages

Netdata’s official RPM repositories are hosted at <https://repository.netdata.cloud/repos/index.html>. We provide four groups of
repositories at that top level:

- `stable`: Contains packages for stable releases of the Netdata Agent.
- `edge`: Contains packages for nightly builds of the Netdata Agent.
- `repoconfig`: Provides packages that set up configuration files for using the other repositories.
- `devel`: Is used for one-off development builds of the Netdata Agent, and can simply be ignored by users.

Within each top level group of repositories, there are directories for each supported group of distributions:

- `amazonlinux`: Is for Amazon Linux and binary compatible distros.
- `el`: Is for Red Hat Enterprise Linux and binary compatible distros that are not covered by other repos, such
  as CentOS, Alma Linux, and Rocky Linux.
- `fedora`: Is for Fedora and binary compatible distros.
- `ol`: Is for Oracle Linux and binary compatible distros.
- `opensuse`: Is for openSUSE and binary compatible distros.

Under each of those directories is a directory for each supported release of that distribution, and under that a
directory for each supported CPU architecture which contains the actual repository.

For example, for stable release packages for RHEL 9 on 64-bit x86, the full URL for the repository would be
<https://repository.netdata.cloud/repos/stable/el/9/x86_64/>

Our RPM packages and repository metadata are signed using a GPG key with a user name of ‘Netdatabot’. The
current key fingerprint is `6588FDD7B14721FE7C3115E6F9177B5265F56346`. The associated public key can be fetched from
`https://repository.netdata.cloud/netdatabot.gpg.key`.

If you are explicitly configuring a system to use our repositories, the recommended setup is to download the
appropriate repository configuration package from <https://repository.netdata.cloud/repos/repoconfig/index.html>
and install it directly on the target system using the system package manager. This will ensure any packages
needed to use the repository are also installed, and will help enable a seamless transition if we ever need to
change our infrastructure.

> **Note**
>
> On RHEL and other systems that use the `el` repositories, some of the dependencies for Netdata can only be found
> in the EPEL repository, which is not enabled or installed by default on most of these systems. This additional
> repository _should_ be pulled in automatically by our repository config packages, but if it is not you may need
> to manually install `epel-release` to be able to successfully install the Netdata packages.

## Manual setup of DEB packages

Netdata’s official DEB repositories are hosted at <https://repository.netdata.cloud/repos/index.html>. We provide four groups of
repositories at that top level:

- `stable`: Contains packages for stable releases of the Netdata Agent.
- `edge`: Contains packages for nightly builds of the Netdata Agent.
- `repoconfig`: Provides packages that set up configuration files for using the other repositories.
- `devel`: Is used for one-off development builds of the Netdata Agent, and can simply be ignored by users.

Within each top level group of repositories, there are directories for each supported group of distributions:

- `debian`: Is for Debian Linux and binary compatible distros.
- `ubuntu`: Is for Ubuntu Linux and binary compatible distros.

Under each of these directories is a directory for each supported release, corresponding to the release codename.

These repositories are set up as what Debian calls ‘flat repositories’, and are available via both HTTP and HTTPS.
Additionally, our repositories support acquiring repository metadata by-hash, which leads to use of URLs similar to
`http://repository.netdata.cloud/repos/edge/ubuntu/focal/by-hash/SHA256/91ccff6523a3c4483ebb539ff2b4adcd3b6b5d0c0c2c9573c5a6947a127819bc`,
and this is the preferred method for updating metadata as it is more reliable.

As a result of this structure, the required APT sources.list entry for stable packages for Debian 11 (Bullseye) is:

```text
deb by-hash=yes http://repository.netdata.cloud/repos/stable/debian/ bullseye/
```

And the equivalent required deb822 style entry for stable packages for Debian 11 (Bullseye) is:

```text
Types: deb
URIs: http://repository.netdata.cloud/repos/stable/debian/
Suites: bullseye/
By-Hash: Yes
Enabled: Yes
```

Note the `/` at the end of the codename, this is required for the repository to be processed correctly.

Our DEB packages and repository metadata are signed using a GPG key with a user name of ‘Netdatabot’. The
current key fingerprint is `6588FDD7B14721FE7C3115E6F9177B5265F56346`. The associated public key can be fetched from
`https://repository.netdata.cloud/netdatabot.gpg.key`.

If you are explicitly configuring a system to use our repositories, the recommended setup is to download the
appropriate repository configuration package from <https://repository.netdata.cloud/repos/repoconfig/index.html> and install it
directly on the target system using the system package manager. This will ensure any packages needed to use the
repository are also installed, and will help enable a seamless transition if we ever need to change our infrastructure.

## Local mirrors of the official Netdata repositories

Local mirrors of our official repositories can be created in one of two ways:

1. Using the standard tooling for mirroring the type of repository you want a local mirror of, such as Aptly for
   APT repositories, or reposync for RPM repositories. For this approach, please consult the documentation for
   the specific tool you are using for info on how to mirror the repositories.
2. Using a regular website mirroring tool, such as GNU wget’s `--mirror` option. For this approach, simply point
   your mirroring tool at `https://repository.netdata.cloud/repos/`, and everything should just work.

We do not provide official support for mirroring our repositories, but we do have some tips for anyone looking to do so:

- Excluding special cases of caching proxies (such as apt-cacher-ng), our repository configuration packages _DO NOT_
  work with custom local mirrors. Thus, you will need to manually configure your systems to use your local mirror.
- Packages are published as they are built, with 64-bit x86 packages being built first, followed by 32-bit x86,
  and then non-x86 packages in alphabetical order of the CPU architecture. Because of the number of different
  packages being built, this means that packages for a given nightly build or stable release are typically published
  over the course of a few hours, usually starting about 15-20 minutes after the build or release is started.
- Repository metadata may be updated as frequently as six times an hour, depending on whether or not there are
  new packages for a given repository. However, it is generally not worth it to attempt to
  synchronize a mirror of our repositories more frequently than once an hour.
- A full mirror of all of our repositories currently requires up to 100 GB of storage space, though the exact
  amount of space needed fluctuates over time. Because of this, users seeking to mirror our repositories are
  encouraged to mirror only those repositories they actually need instead of mirroring everything.
- If syncing daily (or less frequently), some time between 05:00 and 08:00 UTC each day is usually the safest
  time to do so, as publishing nightly packages will almost always be done by this point, and publishing of stable
  releases typically happens after that time window.
- If you intend to use our existing GPG signatures on the repository metadata and packages, you probably also want
  a local copy of our public GPG key, which can be fetched from `https://repository.netdata.cloud/netdatabot.gpg.key`.

## Public mirrors of the official Netdata repositories

There are no official public mirrors of our repositories.

If you wish to provide a public mirror of our official repositories, you are free to do so, but we kindly ask that
you make it clear to your users that your mirror is not an official mirror of our repositories.
