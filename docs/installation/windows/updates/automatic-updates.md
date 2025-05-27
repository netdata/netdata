# Windows Agent Automatic Updates

## Quick Setup

:::tip 

**What You'll Learn**

How to setup automatic Netdata updates on Windows nodes using PowerShell and Task Scheduler.

:::

### How to setup:

**1. Create `C:\netdata` dir.**

```powershell
New-Item -Path "C:\netdata" -ItemType Directory -Force
```

**2. Create `C:\netdata\netdata-updater.ps1`**

Create the file with this exact content:

```powershell
Invoke-WebRequest https://github.com/netdata/netdata-nightlies/releases/latest/download/netdata-x64.msi -OutFile C:\netdata\netdata-x64.msi
msiexec /qn /i C:\netdata\netdata-x64.msi TOKEN="<CLAIM_TOKEN>" ROOMS="<ROOM_ID>" 
```

:::note 

**Configuration Required**

Replace `<CLAIM_TOKEN>` with your Netdata Cloud claim token and `<ROOM_ID>` with your room identifier.

:::

**3. Create an entry in `Task Scheduler`:**

Configure the task with these specific settings:

**General tab** - check:
- `Run whether user is logged in or not`
- `Run with highest privileges`
- `Configure for: Windows Vista, Windows Server 2008`

**Triggers tab** - add an entry for:
- `Daily`
- `7AM UTC`

**Actions tab**:
- **Program/Script:** `powershell`
- **Arguments:** `-noprofile -executionpolicy bypass -file C:\netdata\netdata-updater.ps1`

This setup will automatically download and install the latest Netdata nightly build daily at 7AM UTC.