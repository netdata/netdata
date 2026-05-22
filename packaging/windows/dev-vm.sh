#!/usr/bin/env bash
#
# Drive a Windows MSYS2/UCRT64 VM from a Linux host without touching
# the Windows desktop. See packaging/windows/LOCAL_DEV_LINUX.md for the
# one-time VM setup (quickemu, sshfs, OpenSSH, host alias).
#
# Usage:
#   packaging/windows/dev-vm.sh build               # full Windows build
#   packaging/windows/dev-vm.sh package             # build then build MSI
#   packaging/windows/dev-vm.sh deps                # install/refresh MSYS2 deps
#   packaging/windows/dev-vm.sh shell               # interactive ucrt64 shell
#   packaging/windows/dev-vm.sh run -- '<ps>'       # arbitrary PowerShell
#   packaging/windows/dev-vm.sh msys -- '<sh>'      # arbitrary UCRT64 bash
#
# Override the SSH alias or repo mount path with env vars:
#   VM_HOST     ssh alias to the VM (default: win11-dev)
#   REPO_IN_VM  drive letter of the host repo as mounted in the VM
#               (default: Z:\). Must end with a backslash.

set -euo pipefail

VM_HOST="${VM_HOST:-win11-dev}"
REPO_IN_VM="${REPO_IN_VM:-Z:\\}"

usage() {
    cat <<EOF >&2
Usage: $0 {build|package|deps|shell|run -- '<ps>'|msys -- '<sh>'}

  build                 Run packaging/windows/build.ps1 in the VM.
  package               Run packaging/windows/package.ps1 (build + MSI).
  deps                  Run packaging/windows/install-dependencies.ps1.
  shell                 Open an interactive MSYS2 UCRT64 shell in the VM.
  run -- '<ps>'         Run a raw PowerShell command in the VM.
  msys -- '<sh>'        Run a raw bash command in a UCRT64 login shell.

Env vars:
  VM_HOST=$VM_HOST
  REPO_IN_VM=$REPO_IN_VM
EOF
}

ps_invoke() {
    local script="$1"
    ssh "$VM_HOST" "powershell -NoProfile -ExecutionPolicy Bypass -File ${REPO_IN_VM}${script}"
}

case "${1:-help}" in
    build)
        ps_invoke "packaging\\windows\\build.ps1"
        ;;
    package)
        ps_invoke "packaging\\windows\\package.ps1"
        ;;
    deps)
        ps_invoke "packaging\\windows\\install-dependencies.ps1"
        ;;
    shell)
        ssh -t "$VM_HOST" "C:\\msys64\\msys2_shell.cmd -ucrt64 -here -no-start -defterm"
        ;;
    run)
        shift
        [ "${1:-}" = "--" ] && shift
        ssh "$VM_HOST" "powershell -NoProfile -ExecutionPolicy Bypass -Command \"$*\""
        ;;
    msys)
        shift
        [ "${1:-}" = "--" ] && shift
        # Force MSYSTEM=UCRT64 so this matches what build.ps1 sets up.
        ssh "$VM_HOST" "C:\\msys64\\usr\\bin\\bash.exe -lc \"MSYSTEM=UCRT64 $*\""
        ;;
    -h|--help|help)
        usage
        exit 0
        ;;
    *)
        usage
        exit 2
        ;;
esac
