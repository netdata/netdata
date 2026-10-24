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
"${repo_root}/packaging/windows/fetch-msys2-installer.py" runtime /msys2-runtime.pkg.tar.zst
${GITHUB_ACTIONS+echo "::endgroup::"}

${GITHUB_ACTIONS+echo "::group::Licenses"}
if [ ! -f "/gpl-3.0.txt" ]; then
    cp "${repo_root}/LICENSE" /gpl-3.0.txt
fi

if [ ! -f "/cloud.txt" ]; then
    curl -o /cloud.txt "https://app.netdata.cloud/LICENSE.txt"
fi
${GITHUB_ACTIONS+echo "::endgroup::"}

${GITHUB_ACTIONS+echo "::group::Copy Files"}
tar -xf /msys2-base.tar.zst -C /opt/netdata/ || exit 1
cp -R /opt/netdata/msys64/* /opt/netdata/ || exit 1
cp "${repo_root}/packaging/windows/copy_files.ps1" /opt/netdata/usr/libexec/netdata/ || exit 1
rm -rf /opt/netdata/msys64/

check_msys2_runtime() {
    local expected="$1" hint="$2" version
    version="$(strings -el /opt/netdata/usr/bin/msys-2.0.dll | grep -A1 '^FileVersion$' | tail -n 1 || true)"
    if [ "${version}" != "${expected}" ]; then
        echo "Bundled msys-2.0.dll is '${version}', expected '${expected}'. ${hint}" >&2
        exit 1
    fi
}

# The installer's msys2-runtime 3.6.10 carries 8fbd980844 ("Cygwin: open: Unlock fdtab before open_with_arch()")
# without its upstream fixes, and netdata dies at startup with an access violation inside msys-2.0.dll. Replace the DLL
# with the one from the msys2-runtime 3.6.11 package, which has them. The versions below follow the pins in
# fetch-msys2-installer.py; if the installer pin moves to another runtime, revisit the override.
check_msys2_runtime "3.6.10-c770e1b9fa537fff9287c1fd40ebc84ac498fcb6" \
    "The MSYS2 installer pin no longer bundles msys2-runtime 3.6.10; update or drop the runtime override."
tar -xf /msys2-runtime.pkg.tar.zst -C /opt/netdata/ usr/bin/msys-2.0.dll || exit 1
check_msys2_runtime "3.6.11-00eb8435b1c498e9211c7a7cee01584d76bd5f21" "The msys2-runtime DLL was not installed."
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
