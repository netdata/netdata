#!/usr/bin/env bash
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
GRAY='\033[0;90m'
NC='\033[0m'

# Execute command with visibility
run() {
  printf >&2 "${GRAY}$(pwd) >${NC} ${YELLOW}"
  printf >&2 "%q " "$@"
  printf >&2 "${NC}\n"
  "$@"
  local exit_code=$?
  if [[ ${exit_code} -ne 0 ]]; then
    echo -e >&2 "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e >&2 "${RED}[ERROR]${NC} Command failed with exit code ${exit_code}: ${YELLOW}$1${NC}"
    echo -e >&2 "${RED}        Full command:${NC} $*"
    echo -e >&2 "${RED}        Working dir:${NC} $(pwd)"
    echo -e >&2 "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    return $exit_code
  fi
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ANSIBLE_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
IAC_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
ENV_FILE="${IAC_DIR}/.netdata-iac-claim.env"

if [[ ! -f "${ENV_FILE}" ]]; then
  echo "Missing ${ENV_FILE}. Create it with claim token/rooms/url." >&2
  exit 1
fi

set -a
# shellcheck disable=SC1090
source "${ENV_FILE}"
set +a

SSH_PUB_KEY="${SSH_PUB_KEY:-$HOME/.ssh/id_rsa.pub}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_rsa}"

if [[ ! -f "${SSH_PUB_KEY}" ]]; then
  echo "Missing SSH public key: ${SSH_PUB_KEY}" >&2
  exit 1
fi

PUB_KEY="$(cat "${SSH_PUB_KEY}")"

WORK_DIR="${SCRIPT_DIR}/work"
IMAGES_DIR="${WORK_DIR}/images"
SEEDS_DIR="${WORK_DIR}/seeds"
DISKS_DIR="${WORK_DIR}/disks"
VM_DISK_SIZE="${VM_DISK_SIZE:-20G}"

run mkdir -p "${IMAGES_DIR}" "${SEEDS_DIR}" "${DISKS_DIR}"

VIRSH=(sudo virsh)
VIRT_INSTALL=(sudo virt-install)

# Ensure libvirt default network exists and is active
if ! "${VIRSH[@]}" net-info default >/dev/null 2>&1; then
  NET_XML="${WORK_DIR}/default-net.xml"
  cat > "${NET_XML}" <<'XML'
<network>
  <name>default</name>
  <forward mode='nat'/>
  <bridge name='virbr0' stp='on' delay='0'/>
  <ip address='192.168.122.1' netmask='255.255.255.0'>
    <dhcp>
      <range start='192.168.122.2' end='192.168.122.254'/>
    </dhcp>
  </ip>
</network>
XML
  run "${VIRSH[@]}" net-define "${NET_XML}"
  run "${VIRSH[@]}" net-start default
  run "${VIRSH[@]}" net-autostart default
else
  if ! "${VIRSH[@]}" net-info default | awk -F': ' '/Active/ {print $2}' | grep -q 'yes'; then
    run "${VIRSH[@]}" net-start default
  fi
fi

VM_OS_LIST=("ubuntu2204" "rocky9")
ALL_OS_LIST=("ubuntu2204" "debian12" "rocky9")
PARENT_OS="ubuntu2204"
CHILD_OS_LIST=("rocky9")

image_url() {
  case "$1" in
    ubuntu2204) echo "https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img" ;;
    rocky9) echo "https://download.rockylinux.org/pub/rocky/9/images/x86_64/Rocky-9-GenericCloud.latest.x86_64.qcow2" ;;
    *) echo "" ;;
  esac
}

os_variant() {
  case "$1" in
    ubuntu2204) echo "ubuntu22.04" ;;
    rocky9) echo "rocky9" ;;
    *) echo "" ;;
  esac
}

uefi_code_path() {
  local candidates=(
    "/usr/share/OVMF/OVMF_CODE.fd"
    "/usr/share/edk2-ovmf/x64/OVMF_CODE.fd"
    "/usr/share/edk2/x64/OVMF_CODE.fd"
    "/usr/share/edk2/x64/OVMF_CODE.4m.fd"
  )
  for p in "${candidates[@]}"; do
    if [[ -f "${p}" ]]; then
      echo "${p}"
      return 0
    fi
  done
  return 1
}

uefi_vars_path() {
  local candidates=(
    "/usr/share/OVMF/OVMF_VARS.fd"
    "/usr/share/edk2-ovmf/x64/OVMF_VARS.fd"
    "/usr/share/edk2/x64/OVMF_VARS.fd"
    "/usr/share/edk2/x64/OVMF_VARS.4m.fd"
  )
  for p in "${candidates[@]}"; do
    if [[ -f "${p}" ]]; then
      echo "${p}"
      return 0
    fi
  done
  return 1
}

vm_name() {
  echo "nd-e2e-ansible-$1"
}

make_seed() {
  local os="$1"
  local seed_dir="${SEEDS_DIR}/${os}"
  local user_data="${seed_dir}/user-data"
  local meta_data="${seed_dir}/meta-data"
  local net_cfg="${seed_dir}/network-config"
  local seed_iso="${seed_dir}/seed.iso"
  run mkdir -p "${seed_dir}"

  cat > "${user_data}" <<'USERDATA'
#cloud-config
hostname: __HOSTNAME__
users:
  - name: ansible
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    ssh_authorized_keys:
      - __PUBKEY__
ssh_pwauth: false
disable_root: true
package_update: true
package_upgrade: false
packages:
  - python3
  - sudo
USERDATA

  run sed -i "s|__PUBKEY__|${PUB_KEY}|" "${user_data}"
  run sed -i "s|__HOSTNAME__|nd-${os}|" "${user_data}"

  cat > "${meta_data}" <<METADATA
instance-id: nd-${os}
local-hostname: nd-${os}
METADATA

  if [[ "${os}" != "rocky9" ]]; then
    cat > "${net_cfg}" <<NETCFG
version: 2
ethernets:
  en:
    match:
      name: "en*"
    dhcp4: true
  eth:
    match:
      name: "eth*"
    dhcp4: true
NETCFG

    run genisoimage -output "${seed_iso}" -volid CIDATA -joliet -rock "${user_data}" "${meta_data}" "${net_cfg}"
  else
    run genisoimage -output "${seed_iso}" -volid CIDATA -joliet -rock "${user_data}" "${meta_data}"
  fi
}

create_vm() {
  local os="$1"
  local name
  name="$(vm_name "${os}")"

  if "${VIRSH[@]}" dominfo "${name}" >/dev/null 2>&1; then
    echo "VM ${name} already exists. Skipping creation." >&2
    return 0
  fi

  local url
  url="$(image_url "${os}")"
  local variant
  variant="$(os_variant "${os}")"
  local base_img="${IMAGES_DIR}/${os}.qcow2"
  local overlay_img="${DISKS_DIR}/${os}.qcow2"
  local seed_iso="${SEEDS_DIR}/${os}/seed.iso"
  local boot_args=()

  if [[ "${os}" == "rocky9" ]]; then
    local uefi_code
    local uefi_vars
    uefi_code="$(uefi_code_path || true)"
    uefi_vars="$(uefi_vars_path || true)"
    if [[ -z "${uefi_code}" || -z "${uefi_vars}" ]]; then
      echo "Missing OVMF firmware for UEFI boot. Install edk2-ovmf." >&2
      exit 1
    fi
    boot_args+=(--boot "loader=${uefi_code},loader.readonly=yes,loader.type=pflash,nvram.template=${uefi_vars}")
  fi

  if [[ ! -f "${base_img}" ]]; then
    run curl -L -o "${base_img}" "${url}"
  fi

  make_seed "${os}"

  if [[ ! -f "${overlay_img}" ]]; then
    run qemu-img create -f qcow2 -F qcow2 -b "${base_img}" "${overlay_img}" "${VM_DISK_SIZE}"
  fi

  run "${VIRT_INSTALL[@]}" \
    --name "${name}" \
    --memory 2048 \
    --vcpus 2 \
    --cpu host \
    --disk "path=${overlay_img},format=qcow2" \
    --disk "path=${seed_iso},device=cdrom" \
    --os-variant "${variant}" \
    "${boot_args[@]}" \
    --network "network=default" \
    --graphics none \
    --noautoconsole \
    --import
}

get_vm_ip() {
  local name="$1"
  local mac
  mac=$("${VIRSH[@]}" domiflist "${name}" | awk '/network/ {print $5; exit}')
  if [[ -z "${mac}" ]]; then
    return 1
  fi
  "${VIRSH[@]}" net-dhcp-leases default | awk -v mac="${mac}" '$0 ~ mac {print $5}' | cut -d/ -f1 | head -n1
}

wait_for_ip() {
  local name="$1"
  local ip=""
  for _ in $(seq 1 60); do
    ip=$(get_vm_ip "${name}")
    if [[ -n "${ip}" ]]; then
      echo "${ip}"
      return 0
    fi
    sleep 5
  done
  return 1
}

wait_for_ssh() {
  local ip="$1"
  local port="${2:-22}"
  for _ in $(seq 1 60); do
    if ssh -p "${port}" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i "${SSH_KEY}" ansible@"${ip}" "echo ok" >/dev/null 2>&1; then
      return 0
    fi
    sleep 5
  done
  return 1
}

gen_uuid() {
  if command -v uuidgen >/dev/null 2>&1; then
    uuidgen
  else
    python3 - <<'PY'
import uuid
print(uuid.uuid4())
PY
  fi
}

wait_for_stream_connection() {
  local ip="$1"
  local port="$2"
  local parent_ip="$3"
  for _ in $(seq 1 24); do
    if ssh -p "${port}" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i "${SSH_KEY}" ansible@"${ip}" "(command -v ss >/dev/null 2>&1 && sudo ss -tn state established || sudo netstat -tn) | grep -Eq \"(${parent_ip}:19999|::ffff:${parent_ip}:19999)\"" >/dev/null 2>&1; then
      return 0
    fi
    sleep 5
  done
  return 1
}

setup_debian_container() {
  local name="nd-e2e-ansible-debian12"
  local port="2222"
  if ! docker ps -a --format '{{.Names}}' | grep -q "^${name}$"; then
    run docker run --privileged --name "${name}" -d --cgroupns=host -p "${port}:22" \
      -v /sys/fs/cgroup:/sys/fs/cgroup:rw --tmpfs /run --tmpfs /run/lock \
      jrei/systemd-debian:12
  else
    if ! docker ps --format '{{.Names}}' | grep -q "^${name}$"; then
      run docker rm -f "${name}"
      run docker run --privileged --name "${name}" -d --cgroupns=host -p "${port}:22" \
        -v /sys/fs/cgroup:/sys/fs/cgroup:rw --tmpfs /run --tmpfs /run/lock \
        jrei/systemd-debian:12
    fi
  fi

  for _ in $(seq 1 30); do
    if docker exec "${name}" systemctl is-system-running >/dev/null 2>&1; then
      break
    fi
    sleep 2
  done

  run docker exec "${name}" bash -c "rm -f /run/nologin"

  run docker exec "${name}" bash -c "DEBIAN_FRONTEND=noninteractive apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y openssh-server sudo python3 iproute2"
  run docker exec "${name}" bash -c "id -u ansible >/dev/null 2>&1 || useradd -m -s /bin/bash ansible"
  run docker exec "${name}" bash -c "usermod -aG sudo ansible"
  run docker exec "${name}" bash -c "passwd -d ansible || true"
  run docker exec "${name}" bash -c "usermod -U ansible || true"
  run docker exec "${name}" bash -c "printf 'ansible ALL=(ALL) NOPASSWD:ALL\\n' > /etc/sudoers.d/90-ansible && chmod 440 /etc/sudoers.d/90-ansible"
  run docker exec "${name}" bash -c "mkdir -p /home/ansible/.ssh && chmod 700 /home/ansible/.ssh"
  run docker exec "${name}" bash -c "printf '%s\n' \"${PUB_KEY}\" > /home/ansible/.ssh/authorized_keys"
  run docker exec "${name}" bash -c "chmod 600 /home/ansible/.ssh/authorized_keys && chown -R ansible:ansible /home/ansible/.ssh"
  run docker exec "${name}" bash -c "systemctl enable --now ssh || systemctl enable --now sshd"

  export IP_debian12="127.0.0.1"
  export PORT_debian12="${port}"
}

get_port() {
  local name="PORT_$1"
  local port="${!name-}"
  if [[ -z "${port}" ]]; then
    port="22"
  fi
  echo "${port}"
}

for os in "${VM_OS_LIST[@]}"; do
  create_vm "${os}"
  state=$("${VIRSH[@]}" domstate "$(vm_name "${os}")" 2>/dev/null | tr -d '\r' || true)
  if [[ "${state}" != "running" ]]; then
    run "${VIRSH[@]}" start "$(vm_name "${os}")"
  fi
  ip=$(wait_for_ip "$(vm_name "${os}")")
  if [[ -z "${ip}" ]]; then
    echo "Failed to get IP for $(vm_name "${os}")" >&2
    exit 1
  fi
  echo "${os} IP: ${ip}" >&2
  if ! wait_for_ssh "${ip}"; then
    echo "SSH not ready for ${os} (${ip})" >&2
    exit 1
  fi
  export "IP_${os}"="${ip}"
  run ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i "${SSH_KEY}" ansible@"${ip}" "command -v python3 >/dev/null 2>&1 || (command -v dnf >/dev/null 2>&1 && sudo dnf -y install python3) || (sudo apt-get update && sudo apt-get install -y python3)"
  done

setup_debian_container

if ! wait_for_ssh "${IP_debian12}" "${PORT_debian12}"; then
  echo "SSH not ready for debian12 (${IP_debian12}:${PORT_debian12})" >&2
  exit 1
fi

INV_DIR="${ANSIBLE_DIR}/inventories/e2e"

SEGMENT_A_KEY="$(gen_uuid)"
run mkdir -p "${INV_DIR}/group_vars"
run mkdir -p "${INV_DIR}/files/global/health.d"
run mkdir -p "${INV_DIR}/files/profiles/child_minimal"
run mkdir -p "${INV_DIR}/files/profiles/parent"

cat > "${INV_DIR}/inventory.yml" <<INV
all:
  hosts:
    ubuntu2204:
      ansible_host: ${IP_ubuntu2204}
      ansible_user: ansible
      ansible_ssh_private_key_file: ${SSH_KEY}
      netdata_profiles: [parent]
    debian12:
      ansible_host: ${IP_debian12}
      ansible_port: ${PORT_debian12}
      ansible_user: ansible
      ansible_ssh_private_key_file: ${SSH_KEY}
      netdata_profiles: [standalone]
    rocky9:
      ansible_host: ${IP_rocky9}
      ansible_user: ansible
      ansible_ssh_private_key_file: ${SSH_KEY}
      netdata_profiles: [child_minimal]
INV

cat > "${INV_DIR}/files/global/health.d/e2e.conf" <<EOF
# E2E placeholder alert overrides
EOF

cat > "${INV_DIR}/files/profiles/child_minimal/netdata.conf" <<EOF
[db]
  mode = ram
  retention = 1200
  update every = 1

[ml]
  enabled = no

[health]
  enabled = no

[web]
  bind to = localhost
EOF

cat > "${INV_DIR}/files/profiles/child_minimal/stream.conf" <<EOF
[stream]
  enabled = yes
  destination = ${IP_ubuntu2204}:19999
  api key = ${SEGMENT_A_KEY}
EOF

cat > "${INV_DIR}/files/profiles/parent/netdata.conf" <<EOF
[web]
  mode = static-threaded
EOF

cat > "${INV_DIR}/files/profiles/parent/stream.conf" <<EOF
[stream]
  enabled = no

[${SEGMENT_A_KEY}]
  enabled = yes
EOF

cat > "${INV_DIR}/group_vars/all.yml" <<VARS
netdata_claim_enabled: true
netdata_claim_token: "${NETDATA_CLAIM_TOKEN}"
netdata_claim_rooms: "${NETDATA_CLAIM_ROOMS}"
netdata_claim_url: "${NETDATA_CLAIM_URL}"
netdata_release_channel: "${NETDATA_RELEASE_CHANNEL:-nightly}"

netdata_managed_files:
  - src: files/global/health.d/e2e.conf
    dest: health.d/e2e.conf
    type: health

netdata_profiles_definitions:
  standalone:
    managed_files: []

  child_minimal:
    managed_files:
      - src: files/profiles/child_minimal/netdata.conf
        dest: netdata.conf
        type: netdata_conf
      - src: files/profiles/child_minimal/stream.conf
        dest: stream.conf
        type: stream_conf

  parent:
    managed_files:
      - src: files/profiles/parent/netdata.conf
        dest: netdata.conf
        type: netdata_conf
      - src: files/profiles/parent/stream.conf
        dest: stream.conf
        type: stream_conf
VARS

export ANSIBLE_HOST_KEY_CHECKING=False
export ANSIBLE_ROLES_PATH="${ANSIBLE_DIR}/roles"
run ansible-playbook -i "${INV_DIR}/inventory.yml" "${ANSIBLE_DIR}/playbooks/netdata.yml"

for os in "${ALL_OS_LIST[@]}"; do
  ip_var="IP_${os}"
  ip="${!ip_var}"
  port="$(get_port "${os}")"
  echo "Validating ${os} (${ip})" >&2
  run ssh -p "${port}" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i "${SSH_KEY}" ansible@"${ip}" "systemctl is-active netdata"
  run ssh -p "${port}" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i "${SSH_KEY}" ansible@"${ip}" "sudo test -f /var/lib/netdata/cloud.d/claimed_id"
  done

parent_ip_var="IP_${PARENT_OS}"
parent_ip="${!parent_ip_var}"
for os in "${CHILD_OS_LIST[@]}"; do
  ip_var="IP_${os}"
  ip="${!ip_var}"
  port="$(get_port "${os}")"
  echo "Validating streaming ${os} -> ${parent_ip}" >&2
  if ! wait_for_stream_connection "${ip}" "${port}" "${parent_ip}"; then
    echo "Streaming connection not detected from ${os} to ${parent_ip}" >&2
    exit 1
  fi
  done

echo "E2E completed." >&2
