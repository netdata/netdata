#!/bin/bash

RUN_AS_SERVICE=0

if [ "${MSYSTEM:-}" != "UCRT64" ]; then
    echo "Expected MSYSTEM=UCRT64 for the Windows compile-and-run helper." >&2
    exit 1
fi

# The UCRT64 build keeps MSYS2 only as the shell/tooling environment. Native
# binaries and their runtime DLLs are built and staged by the canonical scripts.
REPO_ROOT="$(dirname "$(dirname "$(cd "$(dirname "${BASH_SOURCE[0]}")" > /dev/null && pwd -P)")")"

install_dependencies() {
    exec "${REPO_ROOT}/packaging/windows/msys2-dependencies.sh"
}

if [ "${1:-}" = "install" ]; then
    install_dependencies
fi

if [ "${1:-}" = "service" ]; then
    RUN_AS_SERVICE=1
fi

set -eu -o pipefail

"${REPO_ROOT}/packaging/windows/compile-on-windows.sh"

# shellcheck source=../windows/win-build-dir.sh
. "${REPO_ROOT}/packaging/windows/win-build-dir.sh"

echo "Stopping service Netdata..."
sc stop "Netdata" || echo "stop Failed, ok"

if [ "${RUN_AS_SERVICE}" -eq 1 ]; then
  sc delete "Netdata" || echo "delete Failed, ok"
fi

# Remove only stale artifacts from the former MSYS-native install path.
# The service package may subsequently add its separately managed auxiliary
# MSYS environment for scripts and plugins.
rm -f /opt/netdata/usr/bin/msys-*.dll /opt/netdata/usr/libexec/netdata/plugins.d/msys-*.dll

if [ "${RUN_AS_SERVICE}" -eq 1 ]; then
    "${REPO_ROOT}/packaging/windows/package-windows.sh"
else
    /ucrt64/bin/cmake --install "${build}"
fi

# register the event log publisher
cmd.exe //c "$(cygpath -w -a "/opt/netdata/usr/bin/wevt_netdata_install.bat")"

#echo
#echo "Compile with:"
#echo "ninja -v -C \"${build}\" install || ninja -v -C \"${build}\" -j 1"

if [ "${RUN_AS_SERVICE}" -eq 1 ]; then
  echo
  echo "Registering Netdata service..."
  sc create "Netdata" binPath= "$(cygpath.exe -w /opt/netdata/usr/bin/netdata.exe)" start= auto

  echo "Starting Netdata service..."
  sc start "Netdata"

else

  echo "Starting netdata..."

  rm -rf /opt/netdata/var/log/netdata/*.log || echo
  /opt/netdata/usr/bin/netdata -D

fi
