#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Shared by the Linux amd64 DEM publisher and its real-client integration tests.
set -euo pipefail

: "${RUNNER_TEMP:?}" "${GITHUB_PATH:?}"
rclone_pin="$(python3 -c 'import json, sys; p = json.load(open(sys.argv[1])); print(p["version"], p["linux_amd64_sha256"])' \
    "$(dirname "$0")/../../packaging/dem-runtime/rclone.json")"
read -r rclone_version rclone_sha256 <<< "$rclone_pin"
rclone_archive="rclone-v${rclone_version}-linux-amd64.zip"
rclone_workdir="$(mktemp -d "${RUNNER_TEMP}/rclone-download.XXXXXX")"
cleanup() {
    local rclone_status=$?
    if [ "$rclone_status" -ne 0 ]; then
        printf 'rclone installation failed in %s (status %s)\n' "$PWD" "$rclone_status" >&2
    fi
    rm -rf "$rclone_workdir"
    exit "$rclone_status"
}
trap cleanup EXIT

# Only public URLs and task-local paths are traced; no publishing credentials are used.
set -x
curl --fail --location --silent --show-error --proto '=https' --proto-redir '=https' \
    "https://downloads.rclone.org/v${rclone_version}/${rclone_archive}" -o "${rclone_workdir}/${rclone_archive}"
(cd "$rclone_workdir" && printf '%s  %s\n' "$rclone_sha256" "$rclone_archive" | sha256sum --check --strict)
unzip -p "${rclone_workdir}/${rclone_archive}" "rclone-v${rclone_version}-linux-amd64/rclone" > "${rclone_workdir}/rclone"
install -D -m 0755 "${rclone_workdir}/rclone" "${RUNNER_TEMP}/rclone/bin/rclone"
"${RUNNER_TEMP}/rclone/bin/rclone" version
printf '%s\n' "${RUNNER_TEMP}/rclone/bin" >> "$GITHUB_PATH"
