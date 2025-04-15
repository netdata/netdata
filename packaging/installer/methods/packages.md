# Install Netdata Using Native DEB/RPM Packages

Netdata provides pre-built native packages for most DEB- and RPM-based Linux distributions, following our [platform support policy](/docs/netdata-agent/versions-and-platforms.md).

Our [kickstart.sh installer](/packaging/installer/methods/kickstart.md) uses these packages by default on supported platforms.

Add `--native-only` when running `kickstart.sh` to force native packages. The script will fail if native packages aren’t available.

> **Note**  
> Until late 2024, Netdata packages were hosted on Package Cloud. All packages are now provided exclusively from our own repositories.

---

## 🛠️ Manual Setup of RPM Packages

Repositories: [https://repository.netdata.cloud/repos/index.html](https://repository.netdata.cloud/repos/index.html)

Available groups:

| Repo         | Purpose                       |
|--------------|-------------------------------|
| `stable`     | Stable Netdata Agent releases |
| `edge`       | Nightly builds                |
| `repoconfig` | Configuration packages        |
| `devel`      | Dev builds (ignore)           |

Supported distributions:

- `amazonlinux`
- `el` (RHEL, CentOS, AlmaLinux, Rocky Linux)
- `fedora`
- `ol` (Oracle Linux)
- `opensuse`

Example repository for RHEL 9 x86_64:  
[https://repository.netdata.cloud/repos/stable/el/9/x86_64/](https://repository.netdata.cloud/repos/stable/el/9/x86_64/)

GPG Key fingerprint:  
`6E155DC153906B73765A74A99DD4A74CECFA8F4F`

Public key:  
[https://repository.netdata.cloud/netdatabot.gpg.key](https://repository.netdata.cloud/netdatabot.gpg.key)

### Steps

1. Download config package:  
   [https://repository.netdata.cloud/repos/repoconfig/index.html](https://repository.netdata.cloud/repos/repoconfig/index.html)

2. Install it with your package manager:

   ```bash
   # For RHEL/CentOS/Fedora
   sudo rpm -i netdata-repo-*.rpm
   sudo dnf install netdata
   ```

   > **Note**  
   > On RHEL systems, EPEL repository is required.
   > Our config packages handle this automatically — if not, install epel-release manually.

---

## 🛠️ Manual Setup of DEB Packages

Repositories: [https://repository.netdata.cloud/repos/index.html](https://repository.netdata.cloud/repos/index.html)

Available groups:

| Repo         | Purpose                       |
|--------------|-------------------------------|
| `stable`     | Stable Netdata Agent releases |
| `edge`       | Nightly builds                |
| `repoconfig` | Configuration packages        |
| `devel`      | Dev builds (ignore)           |

Supported distributions:

- `debian`
- `ubuntu`

APT source for Debian 11 (Bullseye):

```
deb by-hash=yes http://repository.netdata.cloud/repos/stable/debian/ bullseye/
```

Deb822 format:

```
Types: deb
URIs: http://repository.netdata.cloud/repos/stable/debian/
Suites: bullseye/
By-Hash: Yes
Enabled: Yes
```

GPG Key fingerprint:  
`6E155DC153906B73765A74A99DD4A74CECFA8F4F`

Public key:  
[https://repository.netdata.cloud/netdatabot.gpg.key](https://repository.netdata.cloud/netdatabot.gpg.key)

### Steps

1. Download config package:  
   [https://repository.netdata.cloud/repos/repoconfig/index.html](https://repository.netdata.cloud/repos/repoconfig/index.html)

2. Install it using your package manager:

   ```bash
   # For Debian/Ubuntu
   sudo apt install ./netdata-repo_*.deb
   sudo apt update
   sudo apt install netdata
   ```

---

## 🌐 Local Mirrors of the Official Netdata Repositories

You can mirror Netdata’s repositories:

### Recommended Methods:

| Method           | Use case                              |
|------------------|---------------------------------------|
| Standard tools   | e.g., Aptly (APT) or `reposync` (RPM) |
| Simple mirroring | Use `wget --mirror` or similar tools  |

Mirror root URL:  
[https://repository.netdata.cloud/repos/](https://repository.netdata.cloud/repos/)

---

### Mirror Tips:

- Config packages don’t support custom mirrors — configure mirrors manually.
- Packages are built in stages by architecture.
- Metadata updates up to six times/hour.
- Full mirror can require up to **100 GB**.
- Ideal sync window: **05:00–08:00 UTC**.
- Fetch a GPG key from:  
  [https://repository.netdata.cloud/netdatabot.gpg.key](https://repository.netdata.cloud/netdatabot.gpg.key)

---

## 🌍 Public Mirrors of the Official Netdata Repositories

> **There are no official public mirrors**.

If you wish to provide a public mirror of Netdata repositories:

- You’re free to do so.
- Please clearly state to your users that it is *not* an official mirror.
- Follow best practices for repository mirroring and security.
