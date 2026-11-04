# Using custom CA certificates with Netdata

When streaming over an encrypted connection, exporting metrics to a secure endpoint, collecting metrics from secure
services, or connecting to an on-premises instance of Netdata Cloud, the Netdata Agent needs to be able to verify
the TLS certificate of the remote system to ensure the security of the connection.

When the remote system is using a TLS certificate issued by a public certificate authority, this will work correctly
out of box without a need to configure anything extra. However, if the TLS certificate was issued by a private
CA, the certificate for that private CA must be installed on the system the Netdata Agent is running on for the
connections to succeed.

The exact method of installing a certificate for a private CA depends on the installation type and the underlying
platform:

- For native DEB/RPM packages, [install the certificate in the system certificate store](#installing-certificates-in-the-system-certificate-store-on-linux)
- For static builds on Linux, [see the instructions for using custom certificates with our static builds](#using-custom-certificates-with-our-static-builds)
- For our Docker images, [see the instructions for using custom certificates with our Docker containers](#using-custom-certificates-with-our-docker-images)
- For local builds of Netdata on Linux, [install the certificate in the system certificate store](#installing-certificates-in-the-system-certificate-store-on-linux)
- For Windows, [see the instructions for installing custom certificates for Netdata](#using-custom-certificates-on-windows)

## Installing certificates in the system certificate store on Linux

Exact instructions for installing certificates in the system certificate store on Linux vary based on the
distribution. If instructions for your Linux distribution are not listed below, consult the documentation for your
distribution for instructions.

### Debian, Ubuntu, and derivatives

To install a custom CA certificate in the system certificate store on a Debian or Ubuntu system:

1. Ensure the certificate file to be installed is in PEM or DER format.
2. Copy the certificate file to `/usr/local/share/certificates` with a `.crt` file extension (for example, if
   the certificate file is named `local.pem`, copy it to this directory as `local.crt`). You may need to create
   this directory. The certificate file (and the directory) should have permissions set such that all users can
   read the file, but only the root user can write to it.
3. Run the command: `sudo update-ca-certificates`.

### Red Hat Enterprise Linux, Fedora, and derivatives

To install a custom CA certificate in the system certificate store on a Red Hat Enterprise Linux or Fedora system:

1. Ensure the certificate file to be installed is in PEM or DER format.
2. Copy the certificate file to `/etc/pki/ca-trust/source/anchors` with a `.crt` file extension (for example,
   if the certificate file is named `local.pem`, copy it to this directory as `local.crt`). The certificate file
   should have permissions set such that all users can read the file, but only the root user can write to it.
3. Run the command: `sudo update-ca-trust`

### Suse Linux Enterprise and openSUSE

To install a custom CA certificate in the system certificate store on a Suse Linux Enterprise or openSUSE system:

1. Ensure the certificate file to be installed is in PEM or DER format.
2. Copy the certificate file to `/etc/pki/trust/anchors` with a `.crt` file extension (for example, if the certificate
   file is named `local.pem`, copy it to this directory as `local.crt`). The certificate file should have permissions
   set such that all users can read the file, but only the root user can write to it.
3. Run the command: `sudo update-ca-certificates`

### Arch Linux and derivatives

To install a custom CA certificate in the system certificate store on an Arch Linux system:

1. Ensure the certificate file to be installed is in PEM or DER format.
2. Copy the certificate file to `/etc/ca-certificates/trust-store/anchors` with a `.crt` file extension (for example,
   if the certificate file is named `local.pem`, copy it to this directory as `local.crt`). The certificate file
   should have permissions set such that all users can read the file, but only the root user can write to it.
3. Run the command: `sudo update-ca-trust`

### Alpine Linux

To install a custom CA certificate in the system certificate store on an Alpine Linux system:

1. Install the `ca-certificates` package if it is not already installed.
2. Ensure the certificate file to be installed is in PEM or DER format.
3. Copy the certificate file to `/usr/local/share/certificates` with a `.crt` file extension (for example, if
   the certificate file is named `local.pem`, copy it to this directory as `local.crt`). You may need to create
   this directory. The certificate file (and the directory) should have permissions set such that all users can
   read the file, but only the root user can write to it.
4. Run the command: `sudo update-ca-certificates`.

## Using custom certificates with our static builds

For most users of our static builds, simply installing the required certificate files in the system trust store
[as outlined above](#installing-certificates-in-the-system-certificate-store-on-linux) will be sufficient to get
things working correctly, though the certificates should be installed in the system trust store _before_ installing
Netdata, otherwise they may not work until after the next time the agent is updated.

If you are using one of our static builds and installing the certificates in
the system certificate store does not work, please [open a bug report about it on
GitHub](https://github.com/netdata/netdata/issues/new?template=BUG_REPORT.yml), as this usually indicates that
our static builds are not correctly handling certificates on your system.

## Using custom certificates with our Docker images

The simplest way to use custom certificates with our Docker images is to create a custom Docker image that includes
the required certificate.

A custom Docker image including the required certificate can be created using a Dockerfile similar to the following:

```
FROM netdata/netdata:stable

RUN mkdir -p /usr/local/share/certificates

COPY local.pem /usr/local/share/certificates

RUN update-ca-certificates
```

The `COPY` line should be updated to reflect the actual name of the certificate file to be included. Note that
the certificate must be in PEM or DER format with a `.crt` extension.

## Using custom certificates on Windows

Currently, Netdata does not provide integration for most components with the system certificate store on
Windows. Instead, certificates must be installed into the bundled MSYS2 environment shipped as part of Netdata
using the following instructions:

1. Ensure the certificate file to be installed is in PEM or DER format, with a `.crt` extension.
2. From an elevated (administrator) shell, create the directory
`C:\Program Files\Netdata\etc\pki\ca-trust\source\anchors` if it does not exist, and
copy the certificate file into it.
3. Refresh the certificate store by running `update-ca-trust` inside the bundled MSYS2 environment: start
`C:\Program Files\Netdata\msys2.exe` as administrator and run `update-ca-trust`. From an elevated PowerShell
session the same can be done with one line:
`& "C:\Program Files\Netdata\usr\bin\bash.exe" -lc /usr/bin/update-ca-trust`
The script has no `.exe` extension - it is a shell script provided by the MSYS2 `ca-certificates` package,
and it cannot be run from `cmd.exe` or PowerShell directly.
4. Restart the agent: `Restart-Service Netdata`
5. Check that the store was refreshed: the files under
`C:\Program Files\Netdata\etc\pki\ca-trust\extracted\` and `C:\Program Files\Netdata\usr\ssl\cert.pem`
should have fresh timestamps.

Five things to keep in mind:

- After upgrading the agent, run `update-ca-trust` again. The installer resets the trust-store
files it owns under `C:\Program Files\Netdata\etc\pki`; certificates you copied into
`ca-trust\source\anchors` are left in place, but the derived bundles are rebuilt.
- `go.d.plugin` is a native Go binary and uses the Windows certificate store, not the MSYS2
bundle, unless the collector job sets `tls_ca`. If a job sets `tls_ca`, the file it names must
contain the corporate CA; otherwise remove `tls_ca` so the collector uses the Windows store.
Behind a TLS-intercepting proxy, install the corporate CA in the MSYS2 anchors directory and,
for jobs without `tls_ca`, in `Local Computer` > `Trusted Root Certification Authorities`.
- The agent's C components (Netdata Cloud connection, claiming, streaming, exporting) verify
peers against the OpenSSL bundle maintained by `update-ca-trust`, not the Windows store.
- `ssl skip certificate verification = yes` in the `[stream]` section of `stream.conf` disables
certificate verification on the child-to-parent sender, including unknown issuing CAs. To trust
a private CA, use `CAfile` or `CApath` in the same section instead.
- `INSECURE=1` is a Windows installer property for the Netdata Cloud connection - claiming and
ACLK - not a `stream.conf` option. It relaxes certificate checks for Cloud only, not for
streaming or the collectors, and is not a substitute for installing the certificate.

