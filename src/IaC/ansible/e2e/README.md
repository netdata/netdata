# Ansible E2E tests (libvirt + Docker)

## Scope
- Ubuntu 22.04
- Debian 12
- Rocky 9

## Prerequisites
- libvirt + KVM working.
- Docker working.
- Ansible installed on the controller.
- UEFI firmware for Rocky (package `edk2-ovmf` on Arch/Manjaro).
- Claim token/room for the shared Netdata Cloud Space.

## Tokens
- Create a local env file (not committed): `src/IaC/.netdata-iac-claim.env`
- Required variables:
  - `NETDATA_CLAIM_TOKEN`
  - `NETDATA_CLAIM_ROOMS`
  - `NETDATA_CLAIM_URL`
  - `NETDATA_RELEASE_CHANNEL`

## Test flow (high level)
1. Provision VMs for the 3 OS targets.
2. Add them to the Ansible inventory with per-host profile lists and file-based profile definitions.
3. Run the Ansible playbook with nightly channel.
4. Verify service status + claiming + streaming connection.

## Run
- Script: `src/IaC/ansible/e2e/run.sh`
- It will:
  - create a libvirt NAT network if missing
  - download cloud images
  - create VMs with cloud-init (Ubuntu + Rocky)
  - use a systemd-based Debian 12 container with SSH on port 2222
  - generate a local inventory + profile files in `inventories/e2e/`
  - run the playbook
  - validate service + claim + streaming

## VM sizing
- Default disk size is 20G (override with `VM_DISK_SIZE=...` when running the script).

## Validation checks
- `systemctl status netdata`
- `/var/lib/netdata/cloud.d/claimed_id`
- child TCP connection to parent `:19999` (via `ss`/`netstat`)
- `http://NODE:19999/api/v3/info`

## Notes
- Docker is used for a fast smoke run; libvirt VMs are the source of truth.
- The Debian container runs the standalone profile because Docker and libvirt use separate networks by default.
- E2E automation scripts will live in this folder.
