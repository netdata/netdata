# Install Netdata Using Native DEB/RPM Packages

Netdata provides pre-built native packages for most DEB- and RPM-based Linux distributions, following our [platform support policy](/docs/netdata-agent/versions-and-platforms.md).

Our [kickstart.sh installer](/packaging/installer/methods/kickstart.md) uses these by default on supported platforms.

To force usage of native DEB or RPM packages, add `--native-only` when running the script.

:::note

We previously provided packages via PackageCloud, but our repositories there are no longer updated and should not be used.

:::

## Manual setup of RPM packages

Netdata’s official RPM repositories are hosted at <https://repository.netdata.cloud/repos/index.html>. We provide four groups of
repositories at that top level:

- `stable`: Contains packages for stable releases of the Netdata Agent.
- `edge`: Contains packages for nightly builds of the Netdata Agent.
- `repoconfig`: Provides packages that set up configuration files for using the other repositories.
- `devel`: Is used for one-off development builds of the Netdata Agent, and can simply be ignored by users.

Within each top level group of repositories, there are directories for each supported group of distributions:

- `amazonlinux`: Provides packages for Amazon Linux and binary compatible distros.
- `el`: Provides packages for Red Hat Enterprise Linux and binary compatible distros that are not covered by other repos, such
  as CentOS, Alma Linux, and Rocky Linux.
- `fedora`: Provides packages for Fedora and binary compatible distros.
- `ol`: Provides packages for Oracle Linux and binary compatible distros.
- `opensuse`: Provides packages for openSUSE and binary compatible distros.

Each distribution has a directory for each supported releas, with subdirectories for each supported CPU architecture containing the actual repository.

For example, the stable release for RHEL 9 on 64-bit x86 is at:  
<https://repository.netdata.cloud/repos/stable/el/9/x86_64/>  

Our RPM packages and repository metadata are signed with a GPG key with a username of `Netdatabot` and the fingerprint:  
`6E155DC153906B73765A74A99DD4A74CECFA8F4F`.  
The public key is available at:  
<https://repository.netdata.cloud/netdatabot.gpg.key>

For manual repository setup, download the appropriate config package from:  
<https://repository.netdata.cloud/repos/repoconfig/index.html>  
This will ensure any packages needed to use the repository are also installed, and will help enable a seamless transition if we ever need to change our infrastructure.

:::note

On RHEL and other systems that use the `el` repositories, some Netdata dependencies are in the EPEL repository, which isn’t enabled by default.  
Our config packages normally handle this, but if something goes wrong you may need to install `epel-release` manually.

:::

## Manual setup of DEB packages

Netdata’s official DEB repositories are hosted at <https://repository.netdata.cloud/repos/index.html>.
We provide four groups of repositories at that top level:

- `stable`: Contains packages for stable releases of the Netdata Agent.
- `edge`: Contains packages for nightly builds of the Netdata Agent.
- `repoconfig`: Provides packages that set up configuration files for using the other repositories.
- `devel`: Is used for one-off development builds of the Netdata Agent, and can simply be ignored by users.

Within each top level group of repositories, there are directories for each supported group of distributions:

- `debian`: Provides packages for Debian Linux and binary compatible distros.
- `ubuntu`: Provides packages for Ubuntu Linux and binary compatible distros.

Each directory contains subdirectories for supported releases, named by codename.  

Our repositories use **flat repository** structure (per Debian standards) and are accessible via HTTP and HTTPS. They also support metadata retrieval **by-hash**, which is the preferred metadata update method as it improves reliability.

An example APT sources entry for stable releases for Debian 11 (Bullseye):

```
deb by-hash=yes http://repository.netdata.cloud/repos/stable/debian/ bullseye/
```

And the equivalent deb822 style entry:

```
Types: deb
URIs: http://repository.netdata.cloud/repos/stable/debian/
Suites: bullseye/
By-Hash: Yes
Enabled: Yes
```

Note the `/` at the end of the codename, this is required for the repository to be processed correctly.

Our DEB packages and repository metadata are signed with a GPG key with a username of `Netdatabot` and the fingerprint:  
`6E155DC153906B73765A74A99DD4A74CECFA8F4F`.  
The public key is available at:  
<https://repository.netdata.cloud/netdatabot.gpg.key>

For manual repository setup, download the appropriate config package from:  
<https://repository.netdata.cloud/repos/repoconfig/index.html>  

Install it using your package manager to ensure all dependencies are met and to allow a smooth transition if our infrastructure changes.

## Local mirrors of the official Netdata repositories

Local mirrors of our official repositories can be created in one of two ways:

1. Using the standard tooling for mirroring the type of repository you want a local mirror of, such as Aptly for
   APT repositories, or reposync for RPM repositories. For this approach, please consult the documentation for
   the specific tool you are using for info on how to mirror the repositories.
2. Using a regular website mirroring tool, such as GNU wget’s `--mirror` option. For this approach, simply point
   your mirroring tool at `https://repository.netdata.cloud/repos/`, and everything should just work.

We don’t officially support mirroring our repositories, but here are some tips:

- Our repository config packages **don’t** work with custom mirrors (except caching proxies like `apt-cacher-ng`), so you’ll need to configure mirrors manually.
- Packages are built and published in stages: 64-bit x86 first, then 32-bit x86, followed by other architectures in alphabetical order. Publishing takes a few hours.
- Repository metadata updates up to six times an hour, but syncing more than once per hour isn’t necessary.
- A full mirror requires up to **100 GB** of storage, so mirror only what you need.
- For daily syncing, **05:00–08:00 UTC** is ideal, as nightly packages are usually published by then.
- If using our GPG signatures, grab our public key:
  <https://repository.netdata.cloud/netdatabot.gpg.key>

## Public mirrors of the official Netdata repositories

There are no official public mirrors of our repositories.

If you wish to provide a public mirror of our official repositories, you are free to do so, but we kindly ask that you make it clear to your users that your mirror is not an official mirror of our repositories.
