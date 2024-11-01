# Netdata Windows Installer

Netdata offers a convenient Windows installer for easy setup. This executable provides two distinct installation modes, outlined below.

> **Note**
>
> This feature is currently under beta and only available for Nightly releases, and the installer can be found in our [nightlies repo](https://github.com/netdata/netdata-nightlies). A stable version will be released soon.

## Silent Mode (Command line)

This section provides instructions for installing Netdata in silent mode running `msiexec.exe`, which is ideal for automated deployments.

> **Info**
>
> Run the installer as admin to avoid the Windows prompt.
>
> Silent mode skips displaying license agreements, but requires explicitly accepting them using the arguments `GPLLICENSE=1` and `CLOUDLICENSE=1`.

### Available Options

| Option            | Description                                                                                      |
|-------------------|--------------------------------------------------------------------------------------------------|
| `/qn`             | Enables silent mode installation.                                                                |
| `/i`              | The name of MSI installer                                                                        |
| `GPLLICENSE=1`    | Accepts GPL 3.0 license. This option is mandatory for silent installations.                      |
| `CLOUDLICENSE=1`  | Accepts Netdata Cloud license. This option is mandatory for silent installations.                |
| `INSECURE=1`      | Forces insecure connections, bypassing hostname verification (use only if absolutely necessary). |
| `TOKEN=`          | Sets the Claim Token for your Netdata Cloud Space.                                               |
| `ROOMS=`          | Comma-separated list of Room IDs where you want your node to appear.                             |
| `PROXY=`          | Sets the proxy server address if your network requires one.                                      |

### Example Usage

Connect your Agent to your Netdata Cloud Space with token `<YOUR_TOKEN>` and room `<YOUR_ROOM>`:

```bash
msiexec /qn /i netdata-x64.msi /TOKEN="<YOUR_TOKEN>" ROOMS="<YOUR_ROOM>" GPLLICENSE=1 CLOUDLICENSE=1
```

Replace `<YOUR_TOKEN>` and `<YOUR_ROOM>` with your actual Netdata Cloud Space claim token and room ID, respectively.

> **Note**
>
> The Windows version of Netdata is intended for users on paid plans.

## Uninstalling

To uninstall Netdata, access Windows Control Panel.
