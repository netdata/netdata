# Snap packaging

This repository includes Snap packaging with a top-level `snapcraft.yaml` and helper scripts under `snap/`.

## Preferred build path (Ubuntu, non-root): managed mode (LXD)

Use Snapcraft managed mode by default. Do not pass `--destructive-mode`.

### 1. One-time admin bootstrap

Run once as an admin user:

```bash
sudo snap install lxd
sudo lxd init --auto
sudo usermod -aG lxd ubuntu
sudo snap connect snapcraft:lxd lxd:lxd || true
```

Replace `ubuntu` with your actual build username if different.

### 2. Refresh session and validate access

Run as your normal build user:

```bash
newgrp lxd
id -nG | tr ' ' '\n' | grep '^lxd$'
lxc list
```

Expected:
- `lxd` is listed in your groups.
- `lxc list` succeeds without permissions errors.

### 3. Build snap (managed mode)

From the repository root:

```bash
unset SNAPCRAFT_BUILD_ENVIRONMENT
snapcraft clean
snapcraft pack --verbose
```

### 4. Verify artifact

```bash
ls -1 ./*.snap
```

Expected: at least one `netdata_*.snap` file.

## Destructive mode (root-only)

`--destructive-mode` installs build packages on the host and requires root:

```bash
sudo snapcraft pack --destructive-mode --verbose
```

If you run destructive mode as a non-root user, failures like `Unable to acquire the dpkg frontend lock` are expected.

## Install and smoke test (optional, admin)

```bash
sudo snap install --dangerous --devmode ./netdata_*.snap
sudo snap start netdata.netdata
snap services netdata
snap logs -f netdata.netdata
```

The service listens on port `19999` by default.
