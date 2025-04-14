# Install Netdata on Windows

Netdata provides a simple Windows installer for quick setup.

> Note  
> The Windows Agent is available for users with paid Netdata subscriptions.  
> Free users will have limited functionality.

---

## 🚫 Limitations for Free Users

| Agent Type       | Limitation                             |
|-----------------|----------------------------------------|
| Standalone Agent | UI is locked — No local monitoring    |
| Child Agent      | No monitoring data in parent dashboard when streaming to a Linux-based Netdata parent |

---

## Download the Windows Installer (MSI)

Choose the version that suits your needs:

| Version | Download Link | Recommended For |
|---------|----------------|----------------|
| Stable  | [Download Stable](https://github.com/netdata/netdata/releases/latest/download/netdata-x64.msi) | Most users — stable, well-tested |
| Nightly | [Download Nightly](https://github.com/netdata/netdata-nightlies/releases/latest/download/netdata-x64.msi) | Users who need the latest features and can handle potential bugs |

---

## 🤐 Silent Installation (Command Line)

Use silent mode to deploy Netdata without user interaction (ideal for automation).

> [!TIP]
> Run the command prompt as Administrator.

---

### Installation Command Options

| Option        | Description |
|---------------|-------------|
| `/qn`         | Enables silent mode (no user interaction) |
| `/i`          | Specifies the path to the MSI installer |
| `TOKEN=`      | Claim token from your Netdata Cloud Space |
| `ROOMS=`      | Comma-separated Room IDs for your node |
| `PROXY=`      | (Optional) Proxy address if required |
| `INSECURE=1`  | (Optional) Allow insecure connections (hostname verification disabled) |

---

### Example Command

Install Netdata and connect to your Cloud Space:

```bash
msiexec /qn /i netdata-x64.msi TOKEN="<YOUR_TOKEN>" ROOMS="<YOUR_ROOMS>"
```

Replace:

- `<YOUR_TOKEN>` with your claim token  
- `<YOUR_ROOMS>` with your Room ID(s)

---

### Download & Install in One Command (PowerShell)

```powershell
$ProgressPreference = 'SilentlyContinue'; Invoke-WebRequest https://github.com/netdata/netdata/releases/latest/download/netdata-x64.msi -OutFile "netdata-x64.msi"; msiexec /qn /i netdata-x64.msi TOKEN=<YOUR_TOKEN> ROOMS=<YOUR_ROOMS>
```

---

## 🖥️ Graphical Installation (GUI)

1. Download the `.msi` installer.  
2. Double-click to run it.  
3. Grant Administrator privileges when prompted.  
4. Complete the setup wizard.

---

## Access Netdata Dashboard

After installation, open your browser and go to:

```
http://localhost:19999
```

---

## License Information

By using silent installation, you agree to:

- [GPL-3 License](https://raw.githubusercontent.com/netdata/netdata/refs/heads/master/LICENSE) — Netdata Agent  
- [NCUL1 License](https://app.netdata.cloud/LICENSE.txt) — Netdata Web Interface