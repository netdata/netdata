# Install Netdata with kickstart.sh

`kickstart.sh` is the recommended way to install Netdata.

This installation script works on all major Linux distributions. It automatically detects the best way to install Netdata for your system.

---

## Quick Overview

| Task                    | Command / Location               | Notes                                  |
|------------------------|----------------------------------|----------------------------------------|
| Install Netdata        | Run `kickstart.sh`               | Choose nightly or stable release      |
| Connect to Cloud       | Use claim token                  | Attach node to Netdata Cloud UI       |
| Customize install      | Pass flags to control behavior   | Directory, release, update control    |
| Export config for IaC  | Copy config from Cloud UI        | For automation & Infrastructure as Code|

---

## 1. Run the One-Line Install Command

From your terminal:

<Tabs>
  <TabItem value="wget" label="wget">

<OneLineInstallWget/>

  </TabItem>
  <TabItem value="curl" label="curl">

<OneLineInstallCurl/>

  </TabItem>
</Tabs>

---

## ℹ️ Tip — Choosing Stable or Nightly

Check our [release guide](/docs/netdata-agent/versions-and-platforms.md) to understand the difference between nightly and stable releases.

---

## 2. Connect to Netdata Cloud (Optional)

To claim your node and connect it to Netdata Cloud:

```bash
bash kickstart.sh --claim-token YOUR_CLAIM_TOKEN --claim-rooms YOUR_ROOM_ID
```

---

### 🔍 Where to find your claim token

1. Log in to [Netdata Cloud](https://app.netdata.cloud)
2. Navigate to your Space
3. Go to **Space Settings** → **Nodes**  
4. Click **Add Node** → Copy Claim Token  

<!-- Screenshot Placeholder -->
<!-- ![Claim Token in Netdata Cloud UI](../img/kickstart/claim-token-ui.png) -->

---

## 3. Optional Parameters for kickstart.sh

Use these flags to customize your installation.

---

### Directory Options

| Parameter             | Purpose                           |
|----------------------|-----------------------------------|
| `--install-prefix`   | Custom install directory          |
| `--old-install-prefix` | Clean previous install directory |

---

### Interactivity Control

| Parameter               | Purpose                        |
|------------------------|--------------------------------|
| `--non-interactive`    | No prompts (good for scripts)  |
| `--interactive`        | Force interactive prompts      |

---

### Release Channel

| Parameter              | Result                      |
|-----------------------|-----------------------------|
| `--release-channel`   | `nightly` or `stable`      |
| `--install-version`   | Install specific version    |

---

### Auto-Updates

| Parameter         | Behavior        |
|------------------|-----------------|
| `--auto-update`  | Enable updates  |
| `--no-updates`   | Disable updates |

---

### Netdata Cloud Options

| Parameter        | Usage                                |
|-----------------|--------------------------------------|
| `--claim-token` | Provide claim token                 |
| `--claim-rooms` | Assign node to specific Cloud Rooms |

---

## 4. Uninstall or Reinstall Netdata

| Command                 | Result                          |
|------------------------|---------------------------------|
| `--reinstall`          | Reinstall existing Netdata      |
| `--uninstall`          | Uninstall Netdata completely    |

---

## 5. Verify Download Integrity (Optional)

Check if the downloaded script is valid:

```bash
[ "@KICKSTART_CHECKSUM@" = "$(curl -Ss https://get.netdata.cloud/kickstart.sh | md5sum | cut -d ' ' -f 1)" ] && echo "OK, VALID" || echo "FAILED, INVALID"
```

---

## 🧩 What does kickstart.sh actually do?

1. Detects your OS and environment
2. Checks for an existing Netdata installation
3. Installs using:
   - Native packages (preferred)
   - Static build (fallback)
   - Build from source (last resort)
4. Installs an auto-update cron job (unless disabled)
5. Optionally connects your node to Netdata Cloud

---

## Notes & Best Practices

- Stop the Agent with `sudo systemctl stop netdata` before reinstalling
- Customize install location or behavior with flags
- Always verify the downloaded script for security
- Use the `--non-interactive` flag in CI/CD pipelines

---

## Related Docs

- [Connect to Netdata Cloud](/docs/netdata-cloud/connect-agent-to-cloud)
- [Release Channels & Versions](/docs/netdata-agent/versions-and-platforms.md)
- [Uninstall Guide](/docs/netdata-agent/installation/uninstall)
- [Offline Installation Guide](/packaging/installer/methods/offline.md)
```