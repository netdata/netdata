#!/bin/bash

repo_root="$(dirname "$(dirname "$(cd "$(dirname "${BASH_SOURCE[0]}")" > /dev/null && pwd -P)")")"

# shellcheck source=./win-build-dir.sh disable=SC1091
. "${repo_root}/packaging/windows/win-build-dir.sh"

set -eu -o pipefail

# Regenerate keys everytime there is an update
if [ -d /opt/netdata/etc/pki/ ]; then
    rm -rf /opt/netdata/etc/pki/
fi

# Remove previous installation of msys2 script
if [ -f /opt/netdata/usr/bin/bashbug ]; then
    rm -rf /opt/netdata/usr/bin/bashbug
fi

${GITHUB_ACTIONS+echo "::group::Installing"}
# shellcheck disable=SC2154  # build is assigned by win-build-dir.sh
cmake --install "${build}"
${GITHUB_ACTIONS+echo "::endgroup::"}

# Always run: it keeps a cached file that matches the pin and replaces any other.
${GITHUB_ACTIONS+echo "::group::Fetching MSYS2 files"}
"${repo_root}/packaging/windows/fetch-msys2-installer.py" base /msys2-base.tar.zst
"${repo_root}/packaging/windows/fetch-msys2-installer.py" packages /msys2-packages
${GITHUB_ACTIONS+echo "::endgroup::"}

${GITHUB_ACTIONS+echo "::group::Licenses"}
if [ ! -f "/gpl-3.0.txt" ]; then
    cp "${repo_root}/LICENSE" /gpl-3.0.txt
fi

if [ ! -f "/cloud.txt" ]; then
    curl -o /cloud.txt "https://app.netdata.cloud/LICENSE.txt"
fi
${GITHUB_ACTIONS+echo "::endgroup::"}

# The installer base is a snapshot: upgrade it with the packages pinned in fetch-msys2-installer.py, so the MSI ships
# the fixes MSYS2 published after it.
install_msys2_packages() {
    local root=/opt/netdata/msys64 pkgdir=/msys2-packages pkg name version installed hook hookdir
    local -a pacman_root=(--root "${root}" --dbpath "${root}/var/lib/pacman")
    local -A pinned=()

    for pkg in "${pkgdir}"/*.pkg.tar.zst; do
        name="$(pacman -Qp "${pkg}")"
        version="${name#* }"
        name="${name% *}"
        if ! installed="$(pacman -Q "${pacman_root[@]}" "${name}")"; then
            echo "The MSYS2 base does not have ${name}; the pins only upgrade packages it bundles." >&2
            exit 1
        fi
        installed="${installed#* }"
        if [ "$(vercmp "${installed}" "${version}")" -ge 0 ]; then
            echo "The MSYS2 base has ${name} ${installed}, not older than the pin ${version}; drop the pin." >&2
            exit 1
        fi
        # There is no chroot on MSYS2, so pacman would run install scripts against the build host.
        if tar -tf "${pkg}" .INSTALL > /dev/null 2>&1; then
            echo "${name} ${version} has an install script, which pacman would run on the build host; review it." >&2
            exit 1
        fi
        pinned["${name}"]="${version}"
    done

    # For the same reason, override each of the tree's hooks with one that never triggers.
    hookdir="$(mktemp -d)"
    for hook in "${root}"/usr/share/libalpm/hooks/*.hook; do
        cat > "${hookdir}/${hook##*/}" << 'HOOK'
[Trigger]
Type = Path
Operation = Install
Target = netdata-disabled-hook

[Action]
When = PreTransaction
Exec = /usr/bin/true
HOOK
    done

    pacman -U "${pacman_root[@]}" --cachedir "${pkgdir}" --hookdir "${hookdir}" --logfile "${hookdir}/pacman.log" \
        --noconfirm "${pkgdir}"/*.pkg.tar.zst
    if grep -F "[ALPM] running '" "${hookdir}/pacman.log" >&2; then
        echo "pacman ran hooks against the build host." >&2
        exit 1
    fi
    rm -rf "${hookdir}"

    for name in "${!pinned[@]}"; do
        installed="$(pacman -Q "${pacman_root[@]}" "${name}")"
        if [ "${installed#* }" != "${pinned[${name}]}" ]; then
            echo "Bundled ${installed}, expected ${name} ${pinned[${name}]}." >&2
            exit 1
        fi
    done
    pacman -Qkk "${pacman_root[@]}" "${!pinned[@]}"
}

${GITHUB_ACTIONS+echo "::group::Copy Files"}
# Upgrade the freshly unpacked base, and replace the package database a previous run left in /opt/netdata, so pacman
# never sees two versions of a package.
rm -rf /opt/netdata/msys64/
tar -xf /msys2-base.tar.zst -C /opt/netdata/ || exit 1
install_msys2_packages
rm -rf /opt/netdata/var/lib/pacman/local/
cp -R /opt/netdata/msys64/* /opt/netdata/ || exit 1
cp "${repo_root}/packaging/windows/copy_files.ps1" /opt/netdata/usr/libexec/netdata/ || exit 1
rm -rf /opt/netdata/msys64/
${GITHUB_ACTIONS+echo "::endgroup::"}

${GITHUB_ACTIONS+echo "::group::Configure Editor"}
if [ -f "/opt/netdata/etc/profile" ]; then
    echo 'EDITOR="/usr/bin/nano.exe"' >> /opt/netdata/etc/profile
fi
${GITHUB_ACTIONS+echo "::endgroup::"}

# TODO: We will have a PR to adjust CAB file creation and sign. This is only adding necessary structure
#${GITHUB_ACTIONS+echo "::group::CAB file"}
#mkdir "${build}/driver"
#cp "${build}/usr/bin/netdata_driver.*" "${build}/driver"
#powershell.exe -ExecutionPolicy Bypass -File "${repo_root}/packaging/windows/generate-driver-catalog.ps1"
#${GITHUB_ACTIONS+echo "::endgroup::"}
