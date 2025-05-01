# Centralizing systemd-journal Namespace Logs to a Central systemd-journal-remote Server

This guide explains how to centralize `systemd-journald` logs from journal *namespaces* to a remote server using `systemd-journal-upload`, particularly for distributions running systemd versions **prior to 254**, which lack the `--namespace` option.

## Background

`systemd-journal-upload` supports forwarding logs using the `--merge` option. However, it does **not** merge logs across journal namespaces. Each namespace stores its logs in a separate directory, and must be uploaded independently.

Systemd version 254+ introduces `--namespace=NAMESPACE`, which simplifies the process. Until such versions are widely available, the recommended method is to run `systemd-journal-upload` per namespace using `--directory`.

## Assumptions

This setup assumes that users have already:
- Configured `systemd-journal-upload` properly on the local machine
- Enabled and configured `systemd-journal-remote` on the remote server

The namespace uploader uses the same destination as the default `systemd-journal-upload`, so logs from all namespaces will be **multiplexed with the system logs** on the central log server.

## Solution Overview

This solution creates a dedicated `systemd-journal-upload@<namespace>` unit per namespace. A helper script automatically locates the appropriate journal directory for the namespace and launches `systemd-journal-upload` with the correct `--directory` and `--save-state` options.

### Benefits:
- Minimal configuration per namespace
- Proper use of systemd's `StateDirectory=` for persistent state
- No need to hardcode journal paths or state files

---

## 1. Systemd Unit Template

**Path:** `/etc/systemd/system/systemd-journal-upload@.service`

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

### Notes:
- `%i` is the unescaped instance name (e.g., `netdata`)
- `StateDirectory=` ensures state files are handled properly per instance

---

## 2. Helper Script

**Path:** `/usr/local/bin/start-journal-upload-namespace.sh`

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

---

## 3. Usage Example

Assume a namespace called `netdata` exists. To start the upload service for it:

```bash
sudo systemctl enable --now systemd-journal-upload@netdata.service
```

## 4. On the Remote Server

Ensure the central system is running `systemd-journal-remote` with appropriate configuration to receive logs from upload clients. All uploaded logs, including those from namespaces, will appear in the same journal unless further filtering is applied.

---

## Future Improvements

With systemd **254+**, `systemd-journal-upload` supports the `--namespace=` option. This allows uploading logs from specific namespaces without needing to use `--directory=` manually. Furthermore, passing `--namespace=*` enables uploading logs from **all namespaces**, including the default one, interleaved:

```bash
systemd-journal-upload --namespace=*
```

This greatly simplifies deployment and eliminates the need to run multiple `systemd-journal-upload` instances per namespace. It mirrors the functionality introduced in `journalctl` in systemd version 245, now extended to uploading logs in version 254.

Until systemd 254+ becomes widespread, the per-namespace `--directory` approach remains the most compatible and robust method.

---

## See Also
- `man systemd-journal-upload`
- `man systemd-journald`
- `man systemd.exec`
- https://www.freedesktop.org/wiki/Software/systemd/journal-remote/

