# Centralizing systemd-journal Namespace Logs to a Central systemd-journal-remote Server

This guide explains how to forward `systemd-journald` logs from journal _namespaces_ to a remote server using `systemd-journal-upload`, particularly for distributions running systemd versions **prior to 254**, which lack native namespace support.

## Current Limitations

While `systemd-journal-upload` can forward logs with the `--merge` option, it doesn't natively consolidate logs across different journal namespaces in older systemd versions. Each namespace stores logs in a separate directory and requires independent uploading.

Starting with systemd 254, the new `--namespace=NAMESPACE` option simplifies this process. Until this version becomes widely available, we need to run separate `systemd-journal-upload` instances for each namespace using the `--directory` option.

## Prerequisites

This guide assumes you have:

- Configured `systemd-journal-upload` on your local machine
- Set up and enabled `systemd-journal-remote` on your central server

The solution will send namespace logs to the same destination as your regular system logs, **multiplexing** them on your central log server.

## Solution

We'll create a dedicated `systemd-journal-upload@<namespace>` unit for each namespace. A helper script automatically locates the appropriate journal directory and launches `systemd-journal-upload` with the correct parameters.

**Advantages**:

- Minimal per-namespace configuration
- Proper state management using systemd's `StateDirectory=`
- Dynamic journal path detection

### 1. Systemd Unit Template

Create this file at:` /etc/systemd/system/systemd-journal-upload@.service`

<details open><summary>systemd-journal-upload@.service</summary>

```ini
[Unit]
Description=Journal Remote Upload Service for %I Namespace
Documentation=man:systemd-journal-upload(8) file:/usr/local/bin/start-journal-upload-namespace.sh
After=systemd-journald.service network-online.target
Wants=network-online.target

[Service]
User=systemd-journal-upload
SupplementaryGroups=systemd-journal systemd-journal-remote
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=yes
StateDirectory=systemd/journal-upload.%i
StateDirectoryMode=0700
ExecStart=/usr/local/bin/start-journal-upload-namespace.sh %I
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

</details>

Note that `%i` represents the unescaped instance name (e.g., netdata), and `StateDirectory=` ensures proper state file management per namespace.

### 2. Helper Script

Create this file at: `/usr/local/bin/start-journal-upload-namespace.sh`

<details open><summary>start-journal-upload-namespace.sh</summary>

```bash
#!/bin/bash
set -euo pipefail

if [[ $# -lt 1 || -z "$1" || "$1" == "--debug" ]]; then
  echo "Usage: $0 <namespace_name> [--debug]" >&2
  exit ${SYSTEMD_EXIT_CODE_CONFIG:-78}
fi
NAMESPACE_NAME="$1"
shift

DEBUG_MODE=0
[[ "${1:-}" == "--debug" ]] && DEBUG_MODE=1

JOURNAL_BASE_DIR="/var/log/journal"
NAMESPACE_SUFFIX=".$NAMESPACE_NAME"
UPLOADER_CMD="/lib/systemd/systemd-journal-upload"

mapfile -t dirs < <(find "$JOURNAL_BASE_DIR" -maxdepth 1 -mindepth 1 -type d -name "*${NAMESPACE_SUFFIX}")

if [[ ${#dirs[@]} -eq 0 ]]; then
  echo "ERROR: No *${NAMESPACE_SUFFIX} journal directory found." >&2
  exit ${SYSTEMD_EXIT_CODE_CONFIG:-78}
elif [[ ${#dirs[@]} -gt 1 ]]; then
  echo "ERROR: Multiple *${NAMESPACE_SUFFIX} journal directories found:" >&2
  printf "  %s\n" "${dirs[@]}" >&2
  exit ${SYSTEMD_EXIT_CODE_CONFIG:-78}
fi

NAMESPACE_JOURNAL_DIR="${dirs[0]}"

if [[ -z "${STATE_DIRECTORY:-}" ]]; then
  echo "ERROR: \$STATE_DIRECTORY not set. Ensure 'StateDirectory=' is used in the unit." >&2
  [[ $DEBUG_MODE -eq 0 ]] && exit ${SYSTEMD_EXIT_CODE_CONFIG:-78} || STATE_FILE_PATH="\$STATE_DIRECTORY/state (variable not set)"
else
  STATE_FILE_PATH="${STATE_DIRECTORY}/state"
fi

cmd_args=("--directory=$NAMESPACE_JOURNAL_DIR")
[[ -n "${STATE_DIRECTORY:-}" ]] && cmd_args+=("--save-state=$STATE_FILE_PATH")

if [[ $DEBUG_MODE -eq 1 ]]; then
  printf -v cmd_string "%q " "$UPLOADER_CMD" "${cmd_args[@]}"
  echo "[DEBUG] Would execute: $cmd_string"
  exit 0
else
  [[ -z "${STATE_DIRECTORY:-}" ]] && exit 1
  exec "$UPLOADER_CMD" "${cmd_args[@]}"
fi
```

</details>

Remember to make the script executable:

```bash
sudo chmod +x /usr/local/bin/start-journal-upload-namespace.sh
```

### 3. Enabling and Starting the Service

To enable and start the upload service for a namespace called `netdata`:

```bash
sudo systemctl enable --now systemd-journal-upload@netdata.service
```

### 4. Remote Server Configuration

Ensure your central server is running `systemd-journal-remote` and is properly configured to receive logs from upload clients. All uploaded logs, including those from namespaces, will appear in the same journal unless you implement additional filtering.

## Future Improvements

With systemd 254 and newer, you can use the built-in `--namespace=` option:

```bash
systemd-journal-upload --namespace=*
```

This single command uploads logs from all namespaces, including the default one, interleaved together. This approach greatly simplifies deployment by eliminating the need for multiple `systemd-journal-upload` instances.

Until systemd 254+ becomes widely adopted, the per-namespace approach described in this guide remains the most reliable method.

## Additional Resources

- [man systemd-journal-upload](https://www.freedesktop.org/software/systemd/man/latest/systemd-journal-upload.service.html)
- [man systemd-journald](https://www.freedesktop.org/software/systemd/man/latest/systemd-journald.service.html)
- [man systemd.exec](https://www.freedesktop.org/software/systemd/man/latest/systemd.exec.html)
