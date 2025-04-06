# Install Netdata using native DEB/RPM packages

For most DEB- or RPM-based Linux distributions, Netdata provides pre-built native packages, following our [platform support policy](/docs/netdata-agent/versions-and-platforms.md).  

Our [kickstart.sh installer](/packaging/installer/methods/kickstart.md) uses these by default on supported platforms. To force native DEB or RPM packages, add `--native-only` when running the script—it will fail if native packages aren’t available.

> **Note**
> 
> Until late 2024 we continued to provide packages via Package Cloud, but we have since then switched to only  providing packages via our repositories.

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

Each distribution has a directory for supported releases, with subdirectories for each CPU architecture containing the actual repository.  

For example, the stable release for RHEL 9 on 64-bit x86 is at:  
<https://repository.netdata.cloud/repos/stable/el/9/x86_64/>  

Our RPM packages and repository metadata are signed with a GPG key (`Netdatabot`), fingerprint:  
`6E155DC153906B73765A74A99DD4A74CECFA8F4F`.  
Public key:  
<https://repository.netdata.cloud/netdatabot.gpg.key>  

For manual repository setup, download the appropriate config package from:  
<https://repository.netdata.cloud/repos/repoconfig/index.html>  
Install it with your package manager to ensure dependencies and smooth updates.  

> **Note:**  
> On RHEL and other `el`-based systems, some Netdata dependencies are in the EPEL repository, which isn’t enabled by default.  
> Our config packages should handle this, but if not, install `epel-release` manually.

## Manual setup of DEB packages

Netdata’s official DEB repositories are hosted at <https://repository.netdata.cloud/repos/index.html>. 
We provide four groups of repositories at that top level:

- `stable`: Contains packages for stable releases of the Netdata Agent.
- `edge`: Contains packages for nightly builds of the Netdata Agent.
- `repoconfig`: Provides packages that set up configuration files for using the other repositories.
- `devel`: Is used for one-off development builds of the Netdata Agent, and can simply be ignored by users.

Within each top level group of repositories, there are directories for each supported group of distributions:

- `debian`: Is for Debian Linux and binary compatible distros.
- `ubuntu`: Is for Ubuntu Linux and binary compatible distros.

Each directory contains subdirectories for supported releases, named by codename.  

Our repositories use **flat repository** structure (per Debian standards) and are accessible via HTTP and HTTPS. They also support metadata retrieval **by-hash**, which improves reliability. Example:  
`http://repository.netdata.cloud/repos/edge/ubuntu/focal/by-hash/SHA256/91ccff6523a3c4483ebb539ff2b4adcd3b6b5d0c0c2c9573c5a6947a127819bc`

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
current key fingerprint is `6E155DC153906B73765A74A99DD4A74CECFA8F4F`. The associated public key can be fetched from
`https://repository.netdata.cloud/netdatabot.gpg.key`.

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