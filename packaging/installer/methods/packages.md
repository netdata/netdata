# Install Netdata Using Native DEB/RPM Packages

:::note

Netdata provides pre-built native packages for most DEB- and RPM-based Linux distributions, following our [platform support policy](/docs/netdata-agent/versions-and-platforms.md).

:::

Install Netdata using our [kickstart.sh installer](/packaging/installer/methods/kickstart.md), which automatically uses native packages on supported platforms.

To ensure the installer only uses native packages, add the `--native-only` option when running `kickstart.sh`.

:::note

Our previous PackageCloud repositories are no longer updated. All packages are now available exclusively from our own repositories.

:::

## Repository Structure Overview

Our repository system follows a structured organization:

```
repository.netdata.cloud/repos/
├── stable/                   # Stable Netdata Agent releases
│   ├── debian/               # For Debian-based distributions
│   │   ├── bullseye/         # Distribution codename directories
│   │   ├── bookworm/
│   │   └── ...
│   ├── ubuntu/               # For Ubuntu-based distributions
│   │   ├── focal/
│   │   ├── jammy/
│   │   └── ...
│   ├── el/                   # For RHEL-based distributions
│   │   ├── 8/                # Version directories
│   │   │   ├── x86_64/       # Architecture directories
│   │   │   ├── aarch64/
│   │   │   └── ...
│   │   ├── 9/
│   │   └── ...
│   └── other distros...
├── edge/                     # Nightly builds (same structure)
├── repoconfig/               # Configuration packages
└── devel/                    # Development builds (ignore)
```

## Manual Setup of RPM Packages

You can find our RPM repositories at: [https://repository.netdata.cloud/repos/index.html](https://repository.netdata.cloud/repos/index.html)

### Available Repository Groups

| Repo         | Purpose                       |
|--------------|-------------------------------|
| `stable`     | Stable Netdata Agent releases |
| `edge`       | Nightly builds                |
| `repoconfig` | Configuration packages        |
| `devel`      | Dev builds (ignore)           |

### Supported Distributions

Within each repository group, you'll find directories for specific distributions:

| Repository Directory | Primary Distribution | Compatible Distributions |
|---------------------|----------------------|--------------------------|
| `amazonlinux` | Amazon Linux | Binary-compatible Amazon Linux based distros |
| `el` | Red Hat Enterprise Linux | CentOS, AlmaLinux, Rocky Linux, and other binary-compatible distros |
| `fedora` | Fedora | Binary-compatible Fedora-based distros |
| `ol` | Oracle Linux | Binary-compatible Oracle Linux based distros |
| `opensuse` | openSUSE | Binary-compatible SUSE-based distros |

### Repository Structure

Each distribution has:
1. Directories for each supported release version
2. Subdirectories for each supported CPU architecture containing the actual packages

**Example:** For RHEL 9 on 64-bit x86, you'll find the stable repository at:  
[https://repository.netdata.cloud/repos/stable/el/9/x86_64/](https://repository.netdata.cloud/repos/stable/el/9/x86_64/)

### Package Signing

Our RPM packages and repository metadata are signed with a GPG key with a username of `Netdatabot` and the fingerprint:  
`6E155DC153906B73765A74A99DD4A74CECFA8F4F`

Download the public key from:  
[https://repository.netdata.cloud/netdatabot.gpg.key](https://repository.netdata.cloud/netdatabot.gpg.key)

### Installation Steps

1. Download the appropriate config package for your distribution:  
   [https://repository.netdata.cloud/repos/repoconfig/index.html](https://repository.netdata.cloud/repos/repoconfig/index.html)

2. Install it with your package manager:

   ```bash
   # For RHEL/CentOS/Fedora
   sudo rpm -i netdata-repo-*.rpm
   sudo dnf install netdata
   ```

   :::note
   
   On RHEL and other `el` repository distributions, some Netdata dependencies are in the EPEL repository. Our config packages typically handle this automatically, but if you encounter issues, install `epel-release` manually.
   
   :::

## Manual Setup of DEB Packages

You can find our DEB repositories at: [https://repository.netdata.cloud/repos/index.html](https://repository.netdata.cloud/repos/index.html)

### Available Repository Groups

| Repo         | Purpose                       |
|--------------|-------------------------------|
| `stable`     | Stable Netdata Agent releases |
| `edge`       | Nightly builds                |
| `repoconfig` | Configuration packages        |
| `devel`      | Dev builds (ignore)           |

### Supported Distributions

Within each repository group, you'll find directories for specific distributions:

- `debian`: For Debian Linux and binary-compatible distributions
- `ubuntu`: For Ubuntu Linux and binary-compatible distributions

### Repository Structure

Our DEB repositories use a **flat repository structure** (per Debian standards) and support **by-hash** metadata retrieval for improved reliability.

Each directory contains subdirectories for supported releases, named by codename (e.g., `bullseye/`, `jammy/`).

:::important

When configuring repository URLs, include the trailing slash (`/`) after the codename. This is required for the repository to be processed correctly.

:::

### Package Signing

Our DEB packages and repository metadata are signed with a GPG key with a username of `Netdatabot` and the fingerprint:  
`6E155DC153906B73765A74A99DD4A74CECFA8F4F`

Download the public key from:  
[https://repository.netdata.cloud/netdatabot.gpg.key](https://repository.netdata.cloud/netdatabot.gpg.key)

### Example Configuration

<details>
<summary>Click to view example APT configuration</summary>
<br/>

Here's an example APT sources entry for Debian 11 (Bullseye) stable releases:

```
deb by-hash=yes http://repository.netdata.cloud/repos/stable/debian/ bullseye/
```

And the equivalent Deb822 format:

```
Types: deb
URIs: http://repository.netdata.cloud/repos/stable/debian/
Suites: bullseye/
By-Hash: Yes
Enabled: Yes
```
</details>

### Installation Steps

1. Download the appropriate config package for your distribution:  
   [https://repository.netdata.cloud/repos/repoconfig/index.html](https://repository.netdata.cloud/repos/repoconfig/index.html)

2. Install it using your package manager:

   ```bash
   # For Debian/Ubuntu
   sudo apt install ./netdata-repo_*.deb
   sudo apt update
   sudo apt install netdata
   ```

## Example: Complete Installation on Ubuntu 22.04 (Jammy)

<details>
<summary>Click to view complete installation example</summary>
<br/>

Here's a complete example of installing Netdata on Ubuntu 22.04 using native packages:

```bash
# Step 1: Download the repository configuration package
wget https://repository.netdata.cloud/repos/repoconfig/ubuntu/jammy/netdata-repo_latest.jammy_all.deb

# Step 2: Install the repository configuration
sudo apt install ./netdata-repo_latest.jammy_all.deb

# Step 3: Update package lists
sudo apt update

# Step 4: Install Netdata
sudo apt install netdata

# Step 5: Start and enable Netdata service
sudo systemctl enable --now netdata

# Step 6: Verify installation
curl localhost:19999/api/v1/info
```

After installation, you can access the Netdata dashboard at `http://localhost:19999`.
</details>

## Local Mirrors of the Official Netdata Repositories

You can create local mirrors of our repositories using two main approaches:

### Recommended Mirroring Methods

| Method           | Use case                              | Example |
|------------------|---------------------------------------|---------|
| Standard tools   | For formal repository mirroring | `aptly mirror create netdata-stable http://repository.netdata.cloud/repos/stable/debian/ bullseye/` |
| Simple mirroring | For basic HTTP mirroring | `wget --mirror https://repository.netdata.cloud/repos/` |

### Mirror Root URL

[https://repository.netdata.cloud/repos/](https://repository.netdata.cloud/repos/)

### Important Mirroring Tips

:::important

* **Repository config packages:** These don't support custom mirrors (except caching proxies like `apt-cacher-ng`). Configure mirrors manually.
* **Build process:** Packages are built in stages by architecture (64-bit x86 first, then others). Full publishing takes several hours.
* **Update frequency:** Metadata updates up to six times per hour, but syncing hourly is sufficient.
* **Storage requirements:** A full mirror can require up to **100 GB** of space. Mirror only what you need.
* **Recommended sync time:** For daily syncing, **05:00–08:00 UTC** is ideal, as nightly packages are typically published by then.
* **GPG verification:** If using our GPG signatures, download our public key:  
  [https://repository.netdata.cloud/netdatabot.gpg.key](https://repository.netdata.cloud/netdatabot.gpg.key)

:::

## Public Mirrors of the Official Netdata Repositories

There are currently no official public mirrors of our repositories. If you wish to provide a public mirror of our repositories, you are welcome to do so. 

:::important

Please clearly inform your users that your mirror is not officially supported by Netdata. We recommend following industry best practices for repository mirroring and security.

:::

