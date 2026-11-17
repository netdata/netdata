#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Shared by the Linux amd64 DEM publisher and its real-client integration tests.
set -euo pipefail

: "${RUNNER_TEMP:?}" "${GITHUB_PATH:?}"
rclone_version=1.76.0
rclone_sha256=b00f00b3266c3764413afe599d3ec316f6f43b3e0ea8a0bc1d186d2133e46df7
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
