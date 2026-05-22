# Local Windows builds from a Linux workstation

This walkthrough sets up a Windows 11 KVM virtual machine with MSYS2
UCRT64 so a developer on Linux can run the Netdata Windows build
(`packaging/windows/build.ps1`) locally — and drive the VM from the
host shell (or an LLM assistant like Claude Code) without ever opening
the Windows desktop.

Audience: Linux developers iterating on the UCRT64 migration
(SOW-0033) or any other Windows build change. The goal is to make the
Linux → Windows feedback loop as fast and scriptable as the
Linux → Linux loop.

## Overview

```
+--------------------+              +----------------------+
| Linux host         |   SSH/SFTP   | Windows 11 VM (KVM)  |
| your editor, git,  | <----------> | MSYS2 UCRT64 shell   |
| Claude Code, etc.  |   sshfs      | builds netdata.exe   |
+--------------------+              +----------------------+
        |                                        ^
        | mount the repo over sshfs              |
        | so the VM sees host's files live       |
        +----------------------------------------+
```

The Windows side runs `sshd`, the build, and nothing else. Editing and
LLM-driven actions stay on the Linux side. File sync is one-way live
(sshfs); no rsync pipelines, no manual copies.

## 1. Host prerequisites

Debian/Ubuntu:

```bash
sudo apt install qemu-system-x86 qemu-utils ovmf libvirt-clients \
                 libvirt-daemon-system virt-manager \
                 sshfs openssh-client \
                 quickemu quickget
```

Fedora/Arch will have equivalents; the only non-obvious package is
`quickemu`, available from your distro or
https://github.com/quickemu-project/quickemu.

Verify KVM is enabled:

```bash
lscpu | grep -E 'Virtualization|vmx|svm'
[ -w /dev/kvm ] && echo "KVM OK"
```

If `/dev/kvm` isn't writable, add yourself to the `kvm` and `libvirt`
groups and log out/in.

Resource budget for the VM:

- **Disk:** 50 GB. Windows itself takes ~25 GB; MSYS2 packages and
  the netdata build tree take another ~10 GB; leave headroom.
- **RAM:** 8 GB minimum for the VM; 16 GB is comfortable. The Rust
  toolchain compile is memory-hungry.
- **CPU:** as many vCPUs as you can spare. CMake parallel builds
  benefit linearly.

## 2. Create the Windows 11 VM

`quickemu` automates the eval-ISO download and the QEMU command line.
A fresh VM takes about an hour to install Windows the first time.

```bash
mkdir -p ~/vms/windows-11 && cd ~/vms/windows-11
quickget windows 11
quickemu --vm windows-11.conf --display sdl
```

The first `quickemu` boot opens a Windows installer. Accept the
defaults, create a **local** user (not a Microsoft account — easier to
script later), and let Windows finish first-boot setup. Set a memorable
password; you'll use it for SSH.

After Windows is installed, shut it down from the Start menu and edit
`~/vms/windows-11/windows-11.conf` to bump resources:

```
guest_os="windows"
disk_img="windows-11/disk.qcow2"
iso="windows-11/Windows11.iso"
ram="12G"
cpu_cores="6"
```

Reboot: `quickemu --vm windows-11.conf --display none` (headless once
SSH is up).

### One-time Windows tweaks

Run these in PowerShell **as Administrator** inside the VM:

```powershell
# Allow long paths (needed by some MSYS2 packages)
New-ItemProperty -Path "HKLM:\SYSTEM\CurrentControlSet\Control\FileSystem" `
    -Name "LongPathsEnabled" -Value 1 -PropertyType DWORD -Force

# Disable automatic restart-after-update so a Windows Update reboot
# doesn't kill a long-running build
sc.exe config wuauserv start= disabled

# Set the time zone to match the host (optional)
Set-TimeZone -Id "UTC"
```

## 3. Enable SSH on the VM

```powershell
# As Administrator:
Add-WindowsCapability -Online -Name OpenSSH.Server~~~~0.0.1.0
Set-Service -Name sshd -StartupType Automatic
Start-Service sshd

# Open the firewall port (rule usually exists already but make sure it's enabled)
Get-NetFirewallRule -Name 'OpenSSH-Server-In-TCP' | Set-NetFirewallRule -Enabled True

# Set PowerShell as the default SSH login shell (so `ssh user@vm "cmd"` runs
# in PowerShell, which is what `build.ps1` expects)
New-ItemProperty -Path "HKLM:\SOFTWARE\OpenSSH" -Name DefaultShell `
    -Value "C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe" `
    -PropertyType String -Force
```

### SSH key auth from the host

On the Linux host:

```bash
# Reuse your existing key or generate one specifically for this VM
ssh-keygen -t ed25519 -f ~/.ssh/win11-dev -N ''
ssh-copy-id -i ~/.ssh/win11-dev.pub <winuser>@<vm-ip>
```

`<vm-ip>` is what `ipconfig` shows inside the VM, typically
`192.168.122.x` if you're using libvirt's default NAT network.

If you created the Windows user as an **Administrator**, ssh-copy-id
writes to the wrong file. Move the key into the admin-specific file in
the VM (PowerShell as admin):

```powershell
$src = "$env:USERPROFILE\.ssh\authorized_keys"
$dst = "C:\ProgramData\ssh\administrators_authorized_keys"
Move-Item -Force $src $dst
icacls $dst /inheritance:r /grant "Administrators:F" /grant "SYSTEM:F"
```

Add a host alias on the Linux side so the rest of the doc is short:

```bash
cat >> ~/.ssh/config <<'EOF'
Host win11-dev
    HostName 192.168.122.42   # <-- your VM's IP
    User myuser               # <-- your Windows username
    IdentityFile ~/.ssh/win11-dev
    ServerAliveInterval 30
EOF
```

Verify:

```bash
ssh win11-dev "Get-ComputerInfo | Select-Object WindowsProductName,OsVersion"
```

## 4. Install MSYS2 inside the VM

You only need to do this once. We use the same `install-dependencies.ps1`
that GitHub Actions uses, which handles the MSYS2 install and the
package set:

```bash
# Copy the repo's installer script into the VM
scp packaging/windows/install-dependencies.ps1 \
    packaging/windows/msys2-dependencies.sh \
    packaging/windows/functions.ps1 \
    win11-dev:C:/Users/myuser/setup/

ssh win11-dev "powershell -ExecutionPolicy Bypass -File C:/Users/myuser/setup/install-dependencies.ps1"
```

This will:
1. Download and install MSYS2 to `C:\msys64`.
2. `pacman -Syuu` to refresh the package database (may run twice due to
   the msys-runtime self-update; the script handles that).
3. Install the build dependencies including `mingw-w64-ucrt-x86_64-rust`
   and the UCRT64 toolchain.

After this, the VM has everything `build.ps1` needs.

## 5. Sync the repo: sshfs from VM to host

You will edit on the Linux host, then have the VM see the same files
live. The cleanest approach is to mount the host's worktree **inside
the VM** over sshfs (yes, the VM mounts the host, not the other way
around).

### Step 5a — enable SSH on the host (if not already)

```bash
sudo systemctl enable --now ssh
```

### Step 5b — install Win-sshfs in the VM

Inside the VM, install **WinFsp** and **SSHFS-Win** from
https://github.com/winfsp/sshfs-win/releases. Both have signed MSI
installers; install both, reboot the VM.

### Step 5c — mount the host repo in the VM

Inside the VM (PowerShell as admin, or run once interactively to cache
credentials):

```powershell
$mountpoint = "Z:"
$host_user  = "vk"
$host_ip    = "192.168.122.1"   # host as seen from the VM
$host_path  = "/home/vk/repos/nd/rwin"

net use $mountpoint \\sshfs.r\${host_user}@${host_ip}\${host_path}
```

Now `Z:\` inside the VM is the live worktree on Linux. Edits on the
host are visible immediately in the VM; the VM's build output drops
straight into the host's `build/` directory.

(Alternative: virtio-fs via libvirt is faster but more involved to
set up. sshfs is good enough for the build loop.)

### Step 5d — build from the VM, see results on the host

From Linux:

```bash
ssh win11-dev "powershell -ExecutionPolicy Bypass -File Z:\packaging\windows\build.ps1"
```

The build runs in the VM. Output binaries land under `build/` inside
the host repo because that's where the VM is writing them. You can
`ls build/netdata.exe` from Linux even though you've never opened the
VM's desktop.

## 6. Drive the loop from Linux (and from LLMs)

The whole point of this setup is that `ssh win11-dev "..."` is your
new build/test primitive. Wrap it in a small script so other tooling
(shell aliases, editor tasks, Claude Code Bash calls) can use it
without juggling SSH syntax.

The wrapper at `packaging/windows/dev-vm.sh` (shipped alongside this
doc) is exactly that primitive. It exposes:

```
dev-vm.sh build               # full Windows build
dev-vm.sh package             # build + MSI
dev-vm.sh deps                # install/refresh MSYS2 deps
dev-vm.sh shell               # interactive UCRT64 shell (-t for tty)
dev-vm.sh run -- '<ps>'       # arbitrary PowerShell
dev-vm.sh msys -- '<sh>'      # arbitrary UCRT64 bash
```

Override the defaults with environment variables:

```
VM_HOST=win11-dev             # ssh alias from ~/.ssh/config
REPO_IN_VM=Z:\\               # drive letter the repo is mounted at
```

Now from Linux (or from an LLM Bash call):

```bash
packaging/windows/dev-vm.sh build           # full Windows build
packaging/windows/dev-vm.sh msys -- "cd /z && cmake --build build --target netdata"
packaging/windows/dev-vm.sh run -- "Get-Service netdata"
```

Output streams back to the host terminal in real time; exit codes
propagate. That's all an LLM needs.

### Allowing LLM assistants to drive the VM

If you want Claude Code (or another assistant) to run these
non-interactively, allow the wrapper in your project settings:

```jsonc
// .claude/settings.json
{
  "permissions": {
    "allow": [
      "Bash(./packaging/windows/dev-vm.sh:*)"
    ]
  }
}
```

The assistant can then call `./packaging/windows/dev-vm.sh build`,
read the resulting compiler errors, edit files on the host, and
re-build — all without leaving the Linux terminal. From the
assistant's perspective the VM is just another subshell.

Things to be aware of when scripting LLM-driven loops:

- **Build duration:** a clean Windows build is ~5-15 minutes. Use
  `Bash(..., run_in_background=true)` for long builds and poll, or
  raise the per-call timeout.
- **Output noisiness:** redirect to a log file in `build/` and tail
  selectively (`./dev-vm.sh msys -- 'tail -200 build/last.log'`) to
  keep assistant context lean.
- **Idempotency:** `build.ps1` reconfigures from scratch each time
  (it deletes the build dir). For tight loops you'll want a
  `--incremental` mode that just runs `cmake --build build` instead.
  Easy to add later.
- **State drift:** if the VM is suspended overnight, its clock drifts
  and `pacman` keys may complain. `quickemu` has `--display none
  --shortcut` patterns for keeping the VM up between sessions.

## 7. Smoke-testing the rust-demo crate

Once the UCRT64 migration progresses past Phase 2, you can flip the
smoke crate back on locally without waiting for CI:

```bash
# Edit compile-on-windows.sh to remove -DENABLE_RUST_DEMO=Off
# Then:
packaging/windows/dev-vm.sh build
packaging/windows/dev-vm.sh msys -- "/z/build/netdata.exe -D 2>&1 | head -50"
```

If the migration is complete, you should see the
`RUST FFI smoke: rust-demo 0.1.0 reports nd_rust_add(2, 3) = 5`
log line from inside the Linux terminal.

## Troubleshooting

**`Permission denied (publickey)` over SSH:**
The admin-key dance in step 3 matters. If the user is an
Administrator on Windows, the authorized key MUST live in
`C:\ProgramData\ssh\administrators_authorized_keys`, with the strict
ACLs shown. Regular users use `~\.ssh\authorized_keys` (looser ACLs
are fine).

**`sshfs` mount disappears after VM reboot:**
The `net use` command's `/persistent:yes` flag re-mounts on logon,
but only if there's an interactive logon. For a fully-headless VM
without auto-logon, run the `net use` from a startup scheduled task
(Task Scheduler → Create Task → Triggers: At log on; Action: net use
…).

**`MSYSTEM=UCRT64` not honoured:**
Reconfirm: the build's `build.ps1` sets `$env:MSYSTEM = 'UCRT64'`
before invoking msysbash. If you bypass that and call
`bash -l compile-on-windows.sh` directly, you must export
`MSYSTEM=UCRT64` yourself first. `NetdataPlatform.cmake` now
hard-fails if it's anything else.

**Build is slow:**
Confirm the VM has enough vCPUs. Inside MSYS2, `nproc` should match
the VM's allocation. CMake's parallel build uses `-j $(nproc)`.

**`ucrt64 not found` in MSYS2:**
The MSYS2 install in step 4 may have hit a flaky pacman mirror.
`pacman -Syuu` inside the VM (under any MSYS2 shell) refreshes and
retries.

## See also

- `packaging/windows/WINDOWS_INSTALLER.md` — end-user installer docs.
- `packaging/windows/build.ps1` — the script the VM ultimately runs.
- `.agents/sow/current/SOW-0033-*-windows-ucrt64-migration.md` —
  context for why we now require `MSYSTEM=UCRT64`.
