
# Netdata Installer

For installation on Microsoft Windows, there is an executable to install and claim Netdata. This binary offers two different
installation modes, which are described in the following sections.

## Graphical User Interface (GUI)

When you double-click the installer, you will be prompted to accept the installation as an administrator since
Netdata will add a service to your system. After this, the installer GUI will guide you through the following steps:

- Welcome: The initial screen provides details about the actions the installer will perform.
- UI License: This screen presents the [Netdata Cloud UI License](src/web/gui/v2/LICENSE.md). You must accept the license to proceed.
- GPL 3 License: You will then be presented with the GPL 3 License that is used across the Netdata software.
- Destination: By default, Netdata will be installed in `C:\Program Files\Netdata`. If you prefer a different directory, 
  you can change it here.
- Installation: At this stage, the software will be installed in the selected directory.
- Claim: If you haven’t claimed your Agent yet, this screen will allow you to
  [claim](src/claim/README.md) it. The following fields are available:
 
    - Token: Your Netdata Cloud token.
    - Rooms: The rooms to which you want to assign your node, separated by commas.
    - Proxy: (Optional) The proxy address used to communicate with Netdata Cloud.
    - Insecure connection: By default, Netdata verifies the host certificate. If this option is selected,
      certificate verification will be disabled.
    - Open terminal: Selecting this option will open the `MSYS2 `terminal at the end of the installation.
- Finish: The installation completes.  

## Silent Mode

For faster installation, the silent mode does not display the 
[cloud license](https://raw.githubusercontent.com/netdata/netdata/master/src/web/gui/v2/LICENSE.md),
nor the [GPL3 License](https://www.gnu.org/licenses/gpl-3.0.txt). However, you must use the `/A` option to indicate that
you accept both licenses before proceeding.

In silent mode, the following options are available without additional arguments:

- `/S`: Runs the installer in silent mode.
- `/A`: Accepts all licenses required by Netdata. Without this option, the installer will not proceed.
- `/D`: Specifies the destination directory.
- `/T`: Opens the `MSYS2` terminal after installation is complete.
- `/I`: Accepts insecure connections, meaning the hostname will not be verified.

Additionally, the following options require arguments:

- `/TOKEN=`: Specifies your Netdata Cloud token.
- `/ROOMS=`: Specifies the rooms to which you want to connect the host. Multiple rooms can be specified by separating
   them with commas.
- `/PROXY=`: (Optional) If your network uses a proxy, you can specify its address here.

Below is an example of how to use the installer to connect to the cloud with token `YYY` and room `ZZZ`:

```sh
netdata-installer.exe /S /A /TOKEN=YYY /ROOMS=ZZZ
```