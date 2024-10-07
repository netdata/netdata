# Netdata Windows Installer

Netdata offers a convenient Windows installer for easy setup. This executable provides two distinct installation modes, outlined below.

> **Note**
>
> This feature is currently only available for Nightly releases, and the installer can be found in our [nightlies repo](https://github.com/netdata/netdata-nightlies)

## Graphical User Interface (GUI)

Double-clicking the installer initiates the setup process. Since Netdata adds a service to your system, you'll need to provide administrator privileges.

The installer will then guide you through these steps:

1. **Welcome**: This screen provides a summary of the actions the installer will perform.
2. **License Agreements**:
    - [Netdata Cloud UI License](/src/web/gui/v2/LICENSE.md): Review and accept the license terms to proceed.
    - [GPLv3 License](/LICENSE): Read the GNU General Public License v3, which governs the Netdata software.
3. **Destination**:  Choose the installation directory. By default, Netdata installs in `C:\Program Files\Netdata`.
4. **Installation**: The installer will copy the necessary files to the chosen directory.
5. **Connecting**: In order to [connect](/src/claim/README.md) your Agent to your Netdata Cloud Space you need to provide the following:
    - **Claim Token**: This is the Claim Token that will be used to connect the Agent to your Space.
    - **Room IDs**: These are the Room IDs where your Agent will be added. Multiple IDs are separated by a comma `,`.
    - **Proxy address**: Enter the address of a proxy server if required for communication with Netdata Cloud.
    - **Insecure connection**: By default, Netdata verifies the server's certificate. Enabling this option bypasses verification (use only if necessary).
    - **Open Terminal**: Select this option to launch the `MSYS2` terminal after installation completes.
6. **Finish**: The installation process is complete!

## Silent Mode (Command line)

This section provides instructions for installing Netdata in silent mode, which is ideal for automated deployments.

> **Info**
>
> In order to have no interaction with the install process, make sure to run the installation file as an administrator. Otherwise the Windows admin prompt popup will appear
>
> Silent mode skips displaying license agreements, but requires explicitly accepting them using the `/A` option.

### Available Options

| Option    | Description                                                                                      |
|-----------|--------------------------------------------------------------------------------------------------|
| `/S`      | Enables silent mode installation.                                                                |
| `/A`      | Accepts all Netdata licenses. This option is mandatory for silent installations.                 |
| `/D`      | Specifies the desired installation directory (defaults to `C:\Program Files\Netdata`).           |
| `/T`      | Opens the `MSYS2` terminal after installation.                                                   |
| `/I`      | Forces insecure connections, bypassing hostname verification (use only if absolutely necessary). |
| `/TOKEN=` | Sets the Claim Token for your Netdata Cloud Space.                                               |
| `/ROOMS=` | Comma-separated list of Room IDs where you want your node to appear.                             |
| `/PROXY=` | Sets the proxy server address if your network requires one.                                      |

### Example Usage

Connect your Agent to your Netdata Cloud Space with token `<YOUR_TOKEN>` and room `<YOUR_ROOM>`:

```bash
netdata-installer.exe /S /A /TOKEN=<YOUR_TOKEN> /ROOMS=<YOUR_ROOM>
```

Replace `<YOUR_TOKEN>` and `<YOUR_ROOM>` with your actual Netdata Cloud Space claim token and room ID, respectively.

> **Note**
>
> If you are running the installation for the first time, MSYS2 will also initialize.

## Uninstalling

You can uninstall Netdata via running the `uninstall.exe`, located under `<YOUR_INSTALL_LOCATION>\Netdata`.
