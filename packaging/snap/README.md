# Snap packaging

This repository now includes Snap packaging under `snap/`.

## Build locally

From the repository root:

```bash
snapcraft --destructive-mode
```

This produces a `netdata_*.snap` artifact in the current directory.

## Install locally

```bash
sudo snap install --dangerous --devmode ./netdata_*.snap
```

## Start and inspect

```bash
sudo snap start netdata.netdata
snap services netdata
snap logs -f netdata.netdata
```

The service listens on port `19999` by default.
