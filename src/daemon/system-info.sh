#!/usr/bin/env sh

# -------------------------------------------------------------------------------------------------
# detect the kernel

KERNEL_NAME="$(uname -s)"
KERNEL_VERSION="$(uname -r)"
ARCHITECTURE="$(uname -m)"

# -------------------------------------------------------------------------------------------------
# detect the virtualization and possibly the container technology

# systemd-detect-virt: https://github.com/systemd/systemd/blob/df423851fcc05cf02281d11aab6aee7b476c1c3b/src/basic/virt.c#L999
# lscpu: https://github.com/util-linux/util-linux/blob/581b77da7aa4a5205902857184d555bed367e3e0/sys-utils/lscpu.c#L52
virtualization_normalize_name() {
  vname="$1"
  case "$vname" in
    "User-mode Linux") vname="uml" ;;
    "Windows Subsystem for Linux") vname="wsl" ;;
  esac

  echo "$vname" | tr '[:upper:]' '[:lower:]' | sed 's/ /-/g'
}

CONTAINER="unknown"
CONT_DETECTION="none"
CONTAINER_IS_OFFICIAL_IMAGE="${NETDATA_OFFICIAL_IMAGE:-false}"

if [ -z "${VIRTUALIZATION}" ]; then
  VIRTUALIZATION="unknown"
  VIRT_DETECTION="none"

  if command -v systemd-detect-virt >/dev/null 2>&1; then
    VIRTUALIZATION="$(systemd-detect-virt -v)"
    VIRT_DETECTION="systemd-detect-virt"
    CONTAINER_DETECT_TMP="$(systemd-detect-virt -c)"
    [ -n "$CONTAINER_DETECT_TMP" ] && CONTAINER="$CONTAINER_DETECT_TMP"
    CONT_DETECTION="systemd-detect-virt"
  elif command -v lscpu >/dev/null 2>&1; then
    VIRTUALIZATION=$(lscpu | grep "Hypervisor vendor:" | cut -d: -f 2 | awk '{$1=$1};1')
    [ -n "$VIRTUALIZATION" ] && VIRT_DETECTION="lscpu"
    [ -z "$VIRTUALIZATION" ] && lscpu | grep -q "Virtualization:" && VIRTUALIZATION="none"
  elif command -v dmidecode >/dev/null 2>&1; then
    VIRTUALIZATION=$(dmidecode -s system-product-name 2>/dev/null | grep "VMware\|Virtual\|KVM\|Bochs")
    [ -n "$VIRTUALIZATION" ] && VIRT_DETECTION="dmidecode"
  fi

  if [ -z "${VIRTUALIZATION}" ] || [ "$VIRTUALIZATION" = "unknown" ]; then
    if [ "${KERNEL_NAME}" = "FreeBSD" ]; then
      VIRTUALIZATION=$(sysctl kern.vm_guest 2>/dev/null | cut -d: -f 2 | awk '{$1=$1};1')
      [ -n "$VIRTUALIZATION" ] && VIRT_DETECTION="sysctl"
    fi
  fi

  if [ -z "${VIRTUALIZATION}" ]; then
    # Output from the command is outside of spec
    VIRTUALIZATION="unknown"
    VIRT_DETECTION="none"
  elif [ "$VIRTUALIZATION" != "none" ] && [ "$VIRTUALIZATION" != "unknown" ]; then
    VIRTUALIZATION=$(virtualization_normalize_name "$VIRTUALIZATION")
  fi
else
  # Passed from outside - probably in docker run
  VIRT_DETECTION="provided"
fi

# -------------------------------------------------------------------------------------------------
# detect containers with heuristics

if [ "${CONTAINER}" = "unknown" ]; then
  if [ -f /proc/1/sched ]; then
    IFS='(, ' read -r process _ </proc/1/sched
    if [ "${process}" = "netdata" ]; then
      CONTAINER="container"
      CONT_DETECTION="process"
    fi
  fi
  # ubuntu and debian supply /bin/running-in-container
  # https://www.apt-browse.org/browse/ubuntu/trusty/main/i386/upstart/1.12.1-0ubuntu4/file/bin/running-in-container
  if /bin/running-in-container >/dev/null 2>&1; then
    CONTAINER="container"
    CONT_DETECTION="/bin/running-in-container"
  fi

  # lxc sets environment variable 'container'
  #shellcheck disable=SC2154
  if [ -n "${container}" ]; then
    CONTAINER="lxc"
    CONT_DETECTION="containerenv"
  fi

  # docker creates /.dockerenv
  # http://stackoverflow.com/a/25518345
  if [ -f "/.dockerenv" ]; then
    CONTAINER="docker"
    CONT_DETECTION="dockerenv"
  fi

  if [ -n "${KUBERNETES_SERVICE_HOST}" ]; then
    CONTAINER="container"
    CONT_DETECTION="kubernetes"
  fi

  if [ "${KERNEL_NAME}" = FreeBSD ] && command -v sysctl && sysctl security.jail.jailed 2>/dev/null | grep -q "1$"; then
    CONTAINER="jail"
    CONT_DETECTION="sysctl"
  fi
fi

# -------------------------------------------------------------------------------------------------
# detect the operating system

# Initially assume all OS detection values are for a container, these are moved later if we are bare-metal

CONTAINER_OS_DETECTION="unknown"
CONTAINER_NAME="unknown"
CONTAINER_VERSION="unknown"
CONTAINER_VERSION_ID="unknown"
CONTAINER_ID="unknown"
CONTAINER_ID_LIKE="unknown"

if [ "${KERNEL_NAME}" = "Darwin" ]; then
  CONTAINER_ID=$(sw_vers -productName)
  CONTAINER_ID_LIKE="macOS"
  CONTAINER_NAME="macOS"
  CONTAINER_VERSION=$(sw_vers -productVersion)
  CONTAINER_OS_DETECTION="sw_vers"
elif [ "${KERNEL_NAME}" = "FreeBSD" ]; then
  CONTAINER_ID="FreeBSD"
  CONTAINER_ID_LIKE="FreeBSD"
  CONTAINER_NAME="FreeBSD"
  CONTAINER_OS_DETECTION="uname"
  CONTAINER_VERSION=$(uname -r)
  KERNEL_VERSION=$(uname -K)
else
  if [ -f "/etc/os-release" ]; then
    eval "$(grep -E "^(NAME|ID|ID_LIKE|VERSION|VERSION_ID)=" </etc/os-release | sed 's/^/CONTAINER_/')"
    CONTAINER_OS_DETECTION="/etc/os-release"
  fi

  # shellcheck disable=SC2153
  if [ "${CONTAINER_NAME}" = "unknown" ] || [ "${CONTAINER_VERSION}" = "unknown" ] || [ "${CONTAINER_ID}" = "unknown" ]; then
    if [ -f "/etc/lsb-release" ]; then
      if [ "${CONTAINER_OS_DETECTION}" = "unknown" ]; then
        CONTAINER_OS_DETECTION="/etc/lsb-release"
      else
        CONTAINER_OS_DETECTION="Mixed"
      fi
      DISTRIB_ID="unknown"
      DISTRIB_RELEASE="unknown"
      DISTRIB_CODENAME="unknown"
      eval "$(grep -E "^(DISTRIB_ID|DISTRIB_RELEASE|DISTRIB_CODENAME)=" </etc/lsb-release)"
      if [ "${CONTAINER_NAME}" = "unknown" ]; then CONTAINER_NAME="${DISTRIB_ID}"; fi
      if [ "${CONTAINER_VERSION}" = "unknown" ]; then CONTAINER_VERSION="${DISTRIB_RELEASE}"; fi
      if [ "${CONTAINER_ID}" = "unknown" ]; then CONTAINER_ID="${DISTRIB_CODENAME}"; fi
    fi
    if [ -n "$(command -v lsb_release 2>/dev/null)" ]; then
      if [ "${CONTAINER_OS_DETECTION}" = "unknown" ]; then
        CONTAINER_OS_DETECTION="lsb_release"
      else
        CONTAINER_OS_DETECTION="Mixed"
      fi
      if [ "${CONTAINER_NAME}" = "unknown" ]; then CONTAINER_NAME="$(lsb_release -is 2>/dev/null)"; fi
      if [ "${CONTAINER_VERSION}" = "unknown" ]; then CONTAINER_VERSION="$(lsb_release -rs 2>/dev/null)"; fi
      if [ "${CONTAINER_ID}" = "unknown" ]; then CONTAINER_ID="$(lsb_release -cs 2>/dev/null)"; fi
    fi
  fi
fi

# If Netdata is not running in a container then use the local detection as the host
HOST_OS_DETECTION="unknown"
HOST_NAME="unknown"
HOST_VERSION="unknown"
HOST_VERSION_ID="unknown"
HOST_ID="unknown"
HOST_ID_LIKE="unknown"

# 'systemd-detect-virt' returns 'none' if there is no hardware/container virtualization.
if [ "${CONTAINER}" = "unknown" ] || [ "${CONTAINER}" = "none" ]; then
  for v in NAME ID ID_LIKE VERSION VERSION_ID OS_DETECTION; do
    eval "HOST_$v=\$CONTAINER_$v; CONTAINER_$v=none"
  done
else
  # Otherwise try and use a user-supplied bind-mount into the container to resolve the host details
  if [ -e "/host/etc/os-release" ]; then
    eval "$(grep -E "^(NAME|ID|ID_LIKE|VERSION|VERSION_ID)=" </host/etc/os-release | sed 's/^/HOST_/')"
    HOST_OS_DETECTION="/host/etc/os-release"
  fi
  if [ "${HOST_NAME}" = "unknown" ] || [ "${HOST_VERSION}" = "unknown" ] || [ "${HOST_ID}" = "unknown" ]; then
    if [ -f "/host/etc/lsb-release" ]; then
      if [ "${HOST_OS_DETECTION}" = "unknown" ]; then
        HOST_OS_DETECTION="/etc/lsb-release"
      else
        HOST_OS_DETECTION="Mixed"
      fi
      DISTRIB_ID="unknown"
      DISTRIB_RELEASE="unknown"
      DISTRIB_CODENAME="unknown"
      eval "$(grep -E "^(DISTRIB_ID|DISTRIB_RELEASE|DISTRIB_CODENAME)=" </etc/lsb-release)"
      if [ "${HOST_NAME}" = "unknown" ]; then HOST_NAME="${DISTRIB_ID}"; fi
      if [ "${HOST_VERSION}" = "unknown" ]; then HOST_VERSION="${DISTRIB_RELEASE}"; fi
      if [ "${HOST_ID}" = "unknown" ]; then HOST_ID="${DISTRIB_CODENAME}"; fi
    fi
  fi
fi

if [ -d "/etc/pve" ] &&
  echo "${KERNEL_VERSION}" | grep -q -- '-pve$' &&
  command -v pveversion >/dev/null 2>&1; then
  HOST_NAME="Proxmox VE"
  HOST_ID="proxmox"
  HOST_ID_LIKE="proxmox"
  HOST_VERSION="$(pveversion | cut -f 2 -d '/')"
  HOST_VERSION_ID="$(echo "${HOST_VERSION}" | cut -f 1 -d '.')"
  HOST_OS_DETECTION="pveversion"
fi

# -------------------------------------------------------------------------------------------------
# Detect information about the CPU

LCPU_COUNT="unknown"
CPU_MODEL="unknown"
CPU_VENDOR="unknown"
CPU_FREQ="unknown"
CPU_INFO_SOURCE="none"

possible_cpu_freq=""
nproc="$(command -v nproc)"
lscpu="$(command -v lscpu)"
lscpu_output=""
dmidecode="$(command -v dmidecode)"
dmidecode_output=""

if [ -n "${lscpu}" ] && lscpu >/dev/null 2>&1; then
  lscpu_output="$(LC_NUMERIC=C ${lscpu} 2>/dev/null)"
  CPU_INFO_SOURCE="lscpu"
  LCPU_COUNT="$(echo "${lscpu_output}" | grep "^CPU(s):" | cut -f 2 -d ':' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  CPU_VENDOR="$(echo "${lscpu_output}" | grep "^Vendor ID:" | cut -f 2 -d ':' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  CPU_MODEL="$(echo "${lscpu_output}" | grep "^Model name:" | cut -f 2 -d ':' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  if grep -q "^lxcfs /proc" /proc/self/mounts 2>/dev/null && count=$(grep -c ^processor /proc/cpuinfo 2>/dev/null); then
    LCPU_COUNT="$count"
  fi
  possible_cpu_freq="$(echo "${lscpu_output}" | grep -F "CPU max MHz:" | cut -f 2 -d ':' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' | grep -o '^[0-9]*')"
  if [ -z "$possible_cpu_freq" ]; then
    possible_cpu_freq="$(echo "${lscpu_output}" | grep -F "CPU MHz:" | cut -f 2 -d ':' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' | grep -o '^[0-9]*')"
  fi
  if [ -z "$possible_cpu_freq" ]; then
    possible_cpu_freq="$(echo "${lscpu_output}" | grep "^Model name:" | grep -Eo "[0-9\.]+GHz" | grep -o "^[0-9\.]*" | awk '{print int($0*1000)}')"
  fi
  [ -n "$possible_cpu_freq" ] && possible_cpu_freq="${possible_cpu_freq} MHz"
elif [ -n "${dmidecode}" ] && dmidecode -t processor >/dev/null 2>&1; then
  dmidecode_output="$(${dmidecode} -t processor 2>/dev/null)"
  CPU_INFO_SOURCE="dmidecode"
  LCPU_COUNT="$(echo "${dmidecode_output}" | grep -F "Thread Count:" | cut -f 2 -d ':' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  CPU_VENDOR="$(echo "${dmidecode_output}" | grep -F "Manufacturer:" | cut -f 2 -d ':' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  CPU_MODEL="$(echo "${dmidecode_output}" | grep -F "Version:" | cut -f 2 -d ':' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  possible_cpu_freq="$(echo "${dmidecode_output}" | grep -F "Current Speed:" | cut -f 2 -d ':' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
else
  if [ -n "${nproc}" ]; then
    CPU_INFO_SOURCE="nproc"
    LCPU_COUNT="$(${nproc})"
  elif [ "${KERNEL_NAME}" = FreeBSD ]; then
    CPU_INFO_SOURCE="sysctl"
    LCPU_COUNT="$(sysctl -n kern.smp.cpus)"
    if ! possible_cpu_freq=$(sysctl -n machdep.tsc_freq 2>/dev/null); then
      possible_cpu_freq=$(sysctl -n hw.model 2>/dev/null | grep -Eo "[0-9\.]+GHz" | grep -o "^[0-9\.]*" | awk '{print int($0*1000)}')
      [ -n "$possible_cpu_freq" ] && possible_cpu_freq="${possible_cpu_freq} MHz"
    fi
  elif [ "${KERNEL_NAME}" = Darwin ]; then
    CPU_INFO_SOURCE="sysctl"
    LCPU_COUNT="$(sysctl -n hw.logicalcpu)"
  elif [ -d /sys/devices/system/cpu ]; then
    CPU_INFO_SOURCE="sysfs"
    # This is potentially more accurate than checking `/proc/cpuinfo`.
    LCPU_COUNT="$(find /sys/devices/system/cpu -mindepth 1 -maxdepth 1 -type d -name 'cpu*' | grep -cEv 'idle|freq')"
  elif [ -r /proc/cpuinfo ]; then
    CPU_INFO_SOURCE="procfs"
    LCPU_COUNT="$(grep -c ^processor /proc/cpuinfo)"
  fi

  if [ "${KERNEL_NAME}" = Darwin ]; then
    CPU_MODEL="$(sysctl -n machdep.cpu.brand_string)"
    if [ "${ARCHITECTURE}" = "x86_64" ]; then
      CPU_VENDOR="$(sysctl -n machdep.cpu.vendor)"
    else
      CPU_VENDOR="Apple"
    fi
    echo "${CPU_INFO_SOURCE}" | grep -qv sysctl && CPU_INFO_SOURCE="${CPU_INFO_SOURCE} sysctl"
  elif uname --version 2>/dev/null | grep -qF 'GNU coreutils'; then
    CPU_INFO_SOURCE="${CPU_INFO_SOURCE} uname"
    CPU_MODEL="$(uname -p)"
    CPU_VENDOR="$(uname -i)"
  elif [ "${KERNEL_NAME}" = FreeBSD ]; then
    if (echo "${CPU_INFO_SOURCE}" | grep -qv sysctl); then
      CPU_INFO_SOURCE="${CPU_INFO_SOURCE} sysctl"
    fi

    CPU_MODEL="$(sysctl -n hw.model)"
  elif [ -r /proc/cpuinfo ]; then
    if (echo "${CPU_INFO_SOURCE}" | grep -qv procfs); then
      CPU_INFO_SOURCE="${CPU_INFO_SOURCE} procfs"
    fi

    CPU_MODEL="$(grep -F "model name" /proc/cpuinfo | head -n 1 | cut -f 2 -d ':' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
    CPU_VENDOR="$(grep -F "vendor_id" /proc/cpuinfo | head -n 1 | cut -f 2 -d ':' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  fi
fi

if [ "${KERNEL_NAME}" = Darwin ] && [ "${ARCHITECTURE}" = "x86_64" ]; then
  CPU_FREQ="$(sysctl -n hw.cpufrequency)"
elif [ -r /sys/devices/system/cpu/cpu0/cpufreq/base_frequency ]; then
  if (echo "${CPU_INFO_SOURCE}" | grep -qv sysfs); then
    CPU_INFO_SOURCE="${CPU_INFO_SOURCE} sysfs"
  fi

  value="$(cat /sys/devices/system/cpu/cpu0/cpufreq/base_frequency)"
  CPU_FREQ="$((value * 1000))"
elif [ -n "${possible_cpu_freq}" ]; then
  CPU_FREQ="${possible_cpu_freq}"
elif [ -r /sys/devices/system/cpu/cpu0/cpufreq/cpuinfo_max_freq ]; then
  if (echo "${CPU_INFO_SOURCE}" | grep -qv sysfs); then
    CPU_INFO_SOURCE="${CPU_INFO_SOURCE} sysfs"
  fi

  value="$(cat /sys/devices/system/cpu/cpu0/cpufreq/cpuinfo_max_freq)"
  CPU_FREQ="$((value * 1000))"
elif [ -r /proc/cpuinfo ]; then
  if (echo "${CPU_INFO_SOURCE}" | grep -qv procfs); then
    CPU_INFO_SOURCE="${CPU_INFO_SOURCE} procfs"
  fi
  value=$(grep "cpu MHz" /proc/cpuinfo 2>/dev/null | grep -o "[0-9]*" | head -n 1 | awk '{printf "%0.f",int($0*1000000)}')
  [ -n "$value" ] && CPU_FREQ="$value"
fi

freq_units="$(echo "${CPU_FREQ}" | cut -f 2 -d ' ')"

case "${freq_units}" in
  GHz)
    value="$(echo "${CPU_FREQ}" | cut -f 1 -d ' ')"
    CPU_FREQ="$((value * 1000 * 1000 * 1000))"
    ;;
  MHz)
    value="$(echo "${CPU_FREQ}" | cut -f 1 -d ' ')"
    CPU_FREQ="$((value * 1000 * 1000))"
    ;;
  KHz)
    value="$(echo "${CPU_FREQ}" | cut -f 1 -d ' ')"
    CPU_FREQ="$((value * 1000))"
    ;;
  *) ;;

esac

# -------------------------------------------------------------------------------------------------
# Detect the total system RAM

TOTAL_RAM="unknown"
RAM_DETECTION="none"

if [ "${KERNEL_NAME}" = FreeBSD ]; then
  RAM_DETECTION="sysctl"
  TOTAL_RAM="$(sysctl -n hw.physmem)"
elif [ "${KERNEL_NAME}" = Darwin ]; then
  RAM_DETECTION="sysctl"
  TOTAL_RAM="$(sysctl -n hw.memsize)"
elif [ -r /proc/meminfo ]; then
  RAM_DETECTION="procfs"
  TOTAL_RAM="$(grep -F MemTotal /proc/meminfo | cut -f 2 -d ':' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' | cut -f 1 -d ' ')"
  TOTAL_RAM="$((TOTAL_RAM * 1024))"
fi

# -------------------------------------------------------------------------------------------------
# Detect the total system disk space

is_inside_lxc_container() {
  mounts_file="/proc/self/mounts"

  [ ! -r "$mounts_file" ] && return 1

  # Check if lxcfs is mounted on /proc
  awk '$1 == "lxcfs" && $2 ~ "^/proc" { found=1; exit } END { exit !found }' "$mounts_file"

  return $?
}

DISK_SIZE="unknown"
DISK_DETECTION="none"

if [ "${KERNEL_NAME}" = "Darwin" ]; then
  if DISK_SIZE=$(diskutil info / 2>/dev/null | awk '/Disk Size/ {total += substr($5,2,length($5))} END { print total }') &&
    [ -n "$DISK_SIZE" ] && [ "$DISK_SIZE" != "0" ]; then
    DISK_DETECTION="diskutil"
  else
    types='hfs'

    if (lsvfs | grep -q apfs); then
      types="${types},apfs"
    fi

    if (lsvfs | grep -q ufs); then
      types="${types},ufs"
    fi

    DISK_DETECTION="df"
    DISK_SIZE=$(($(/bin/df -k -t ${types} | tail -n +2 | sed -E 's/\/dev\/disk([[:digit:]]*)s[[:digit:]]*/\/dev\/disk\1/g' | sort -k 1 | awk -F ' ' '{s=$NF;for(i=NF-1;i>=1;i--)s=s FS $i;print s}' | uniq -f 9 | awk '{print $8}' | tr '\n' '+' | rev | cut -f 2- -d '+' | rev) * 1024))
  fi
elif [ "${KERNEL_NAME}" = FreeBSD ]; then
  types='ufs'

  if (lsvfs | grep -q zfs); then
    types="${types},zfs"
  fi

  DISK_DETECTION="df"
  total="$(df -t ${types} -c -k | tail -n 1 | awk '{print $2}')"
  DISK_SIZE="$((total * 1024))"
else
  if [ -d /sys/block ] && [ -r /proc/devices ] && ! is_inside_lxc_container; then
    dev_major_whitelist=''

    # This is a list of device names used for block storage devices.
    # These translate to the prefixs of files in `/dev` indicating the device type.
    # They are sorted by lowest used device major number, with dynamically assigned ones at the end.
    # We use this to look up device major numbers in `/proc/devices`
    device_names='hd sd mfm ad ftl pd nftl dasd intfl mmcblk mmc ub xvd rfd vbd nvme virtblk blkext'

    for name in ${device_names}; do
      if grep -qE " ${name}\$" /proc/devices; then
        dev_major_whitelist="${dev_major_whitelist}:$(grep -E "${name}\$" /proc/devices | sed -e 's/^[[:space:]]*//' | cut -f 1 -d ' ' | tr '\n' ':'):"
      fi
    done

    DISK_DETECTION="sysfs"
    DISK_SIZE="0"
    for disk in /sys/block/*; do
      if [ -r "${disk}/size" ] &&
        (echo "${dev_major_whitelist}" | grep -q ":$(cut -f 1 -d ':' "${disk}/dev"):") &&
        grep -qv 1 "${disk}/removable"; then
        size="$(($(cat "${disk}/size") * 512))"
        DISK_SIZE="$((DISK_SIZE + size))"
      fi
    done
  elif df --version 2>/dev/null | grep -qF "GNU coreutils"; then
    DISK_DETECTION="df"
    DISK_SIZE=$(($(df -x tmpfs -x devtmpfs -x squashfs -l -B1 --output=source,size | tail -n +2 | sort -u -k 1 | awk '{print $2}' | tr '\n' '+' | head -c -1)))
  else
    DISK_DETECTION="df"
    include_fs_types="ext*|btrfs|xfs|jfs|reiser*|zfs"
    DISK_SIZE=$(($(df -T -P | tail -n +2 | sort -u -k 1 | grep -E "${include_fs_types}" | awk '{print $3}' | tr '\n' '+' | head -c -1) * 1024))
  fi
fi

# -------------------------------------------------------------------------------------------------
# Detect whether the node is kubernetes node

HOST_IS_K8S_NODE="false"

if [ -n "${KUBERNETES_SERVICE_HOST}" ] && [ -n "${KUBERNETES_SERVICE_PORT}" ]; then
  # These env vars are set for every container managed by k8s.
  HOST_IS_K8S_NODE="true"
elif pgrep "kubelet"; then
  # The kubelet is the primary "node agent" that runs on each node.
  HOST_IS_K8S_NODE="true"
fi

# ------------------------------------------------------------------------------------------------
# Detect instance metadata for VMs running on cloud providers

CLOUD_TYPE="unknown"
CLOUD_INSTANCE_TYPE="unknown"
CLOUD_INSTANCE_REGION="unknown"

if [ "${VIRTUALIZATION}" != "none" ] && command -v curl >/dev/null 2>&1; then
  # Returned HTTP status codes: GCP is 200, AWS is 200, DO is 404.
  curl --fail -s -m 1 --noproxy "*" http://169.254.169.254 >/dev/null 2>&1
  ret=$?
  # anything but operation timeout.
  if [ "$ret" != 28 ]; then
    # Try AWS IMDSv2
    if [ "${CLOUD_TYPE}" = "unknown" ]; then
      AWS_IMDS_TOKEN="$(curl --fail -s --connect-timeout 1 -m 3 --noproxy "*" -X PUT "http://169.254.169.254/latest/api/token" -H "X-aws-ec2-metadata-token-ttl-seconds: 21600")"
      if [ -n "${AWS_IMDS_TOKEN}" ]; then
        CLOUD_TYPE="AWS"
        CLOUD_INSTANCE_TYPE="$(curl --fail -s --connect-timeout 1 -m 3 --noproxy "*" -H "X-aws-ec2-metadata-token: $AWS_IMDS_TOKEN" -v "http://169.254.169.254/latest/meta-data/instance-type" 2>/dev/null)"
        CLOUD_INSTANCE_REGION="$(curl --fail -s --connect-timeout 1 -m 3 --noproxy "*" -H "X-aws-ec2-metadata-token: $AWS_IMDS_TOKEN" -v "http://169.254.169.254/latest/meta-data/placement/region" 2>/dev/null)"
      fi
    fi

    # Try GCE computeMetadata v1
    if [ "${CLOUD_TYPE}" = "unknown" ]; then
      if curl --fail -s --connect-timeout 1 -m 3 --noproxy "*" -H "Metadata-Flavor: Google" "http://metadata.google.internal/computeMetadata/v1" | grep -sq computeMetadata; then
        CLOUD_TYPE="GCP"
        CLOUD_INSTANCE_TYPE="$(curl --fail -s --connect-timeout 1 -m 3 --noproxy "*" -H "Metadata-Flavor: Google" "http://metadata.google.internal/computeMetadata/v1/instance/machine-type")"
        [ -n "$CLOUD_INSTANCE_TYPE" ] && CLOUD_INSTANCE_TYPE=$(basename "$CLOUD_INSTANCE_TYPE")
        CLOUD_INSTANCE_REGION="$(curl --fail -s --connect-timeout 1 -m 3 --noproxy "*" -H "Metadata-Flavor: Google" "http://metadata.google.internal/computeMetadata/v1/instance/zone")"
        [ -n "$CLOUD_INSTANCE_REGION" ] && CLOUD_INSTANCE_REGION=$(basename "$CLOUD_INSTANCE_REGION") && CLOUD_INSTANCE_REGION=${CLOUD_INSTANCE_REGION%-*}
      fi
    fi

    # Try Azure IMDS
    if [ "${CLOUD_TYPE}" = "unknown" ]; then
      AZURE_IMDS_DATA="$(curl --fail -s --connect-timeout 1 -m 3 -H "Metadata: true" --noproxy "*" "http://169.254.169.254/metadata/instance?api-version=2021-10-01")"
      if [ -n "${AZURE_IMDS_DATA}" ] && echo "${AZURE_IMDS_DATA}" | grep -sq azEnvironment; then
        CLOUD_TYPE="Azure"
        CLOUD_INSTANCE_TYPE="$(curl --fail -s --connect-timeout 1 -m 3 -H "Metadata: true" --noproxy "*" "http://169.254.169.254/metadata/instance/compute/vmSize?api-version=2021-10-01&format=text")"
        CLOUD_INSTANCE_REGION="$(curl --fail -s --connect-timeout 1 -m 3 -H "Metadata: true" --noproxy "*" "http://169.254.169.254/metadata/instance/compute/location?api-version=2021-10-01&format=text")"
      fi
    fi
  fi
fi

# -------------------------------------------------------------------------------------------------
# Detect the IP address of the interface on the default route

get_default_interface_ip() {
  DEFAULT_INTERFACE_IP="unknown"
  DEFAULT_INTERFACE_NAME="unknown"
  DEFAULT_INTERFACE_DETECTION="none"

  # Optional parameter for IP version: "-4" (default) or "-6"
  ip_version="${1:--4}"

  # Check if timeout command is available
  timeout_cmd=""
  if command -v timeout >/dev/null 2>&1; then
    timeout_cmd="timeout 2"
  fi

  # Find default interface based on OS
  default_if=""

  case "${KERNEL_NAME}" in
    Linux)
      # Ultra-safe: Try /proc first (never hangs - just file reading)
      if [ "${ip_version}" = "-4" ] && [ -r /proc/net/route ]; then
        # Default route has destination 00000000
        default_if=$(awk '$2 == "00000000" && $1 != "lo" {print $1; exit}' /proc/net/route)
        [ -n "${default_if}" ] && DEFAULT_INTERFACE_DETECTION="procfs"
      elif [ "${ip_version}" = "-6" ] && [ -r /proc/net/ipv6_route ]; then
        # IPv6 default route - field 10 is the interface
        # Look for ::/0 route (all zeros with prefix 00) that's not on lo
        default_if=$(awk '$1 == "00000000000000000000000000000000" && $2 == "00" && $10 != "lo" {print $10; exit}' /proc/net/ipv6_route)
        [ -n "${default_if}" ] && DEFAULT_INTERFACE_DETECTION="procfs"
      fi

      # Fallback to ip command if /proc didn't work
      if [ -z "${default_if}" ] && command -v ip >/dev/null 2>&1; then
        if [ "${ip_version}" = "-4" ]; then
          # Extract interface after "dev" keyword
          default_if=$(${timeout_cmd} ip -o -4 route list default 2>/dev/null | \
            awk '{for(i=1;i<=NF;i++) if($i=="dev") {print $(i+1); exit}}')
        else
          default_if=$(${timeout_cmd} ip -o -6 route list default 2>/dev/null | \
            awk '{for(i=1;i<=NF;i++) if($i=="dev") {print $(i+1); exit}}')
        fi
        [ -n "${default_if}" ] && DEFAULT_INTERFACE_DETECTION="iproute2"
      fi

      # Last resort: netstat (if available)
      if [ -z "${default_if}" ] && command -v netstat >/dev/null 2>&1; then
        if [ "${ip_version}" = "-4" ]; then
          default_if=$(${timeout_cmd} netstat -rn 2>/dev/null | \
            awk '$1 == "0.0.0.0" && $2 != "0.0.0.0" {print $NF; exit}')
        else
          default_if=$(${timeout_cmd} netstat -rn -A inet6 2>/dev/null | \
            awk '$1 == "::/0" {print $NF; exit}')
        fi
        [ -n "${default_if}" ] && DEFAULT_INTERFACE_DETECTION="netstat"
      fi
      ;;

    Darwin)
      # macOS specific handling
      if [ "${ip_version}" = "-4" ]; then
        # Try route first
        default_if=$(${timeout_cmd} route -n get default 2>/dev/null | \
          awk '/interface:/ {print $2; exit}')
        [ -n "${default_if}" ] && DEFAULT_INTERFACE_DETECTION="route"

        # Fallback to netstat
        if [ -z "${default_if}" ]; then
          default_if=$(${timeout_cmd} netstat -rnf inet 2>/dev/null | \
            awk '/^default/ {print $4; exit}')
          [ -n "${default_if}" ] && DEFAULT_INTERFACE_DETECTION="netstat"
        fi
      else
        # IPv6 - route doesn't work, use netstat
        # Note: may have multiple defaults (utun interfaces)
        default_if=$(${timeout_cmd} netstat -rnf inet6 2>/dev/null | \
          awk '/^default/ && $4 !~ /^utun/ {print $4; exit}')
        if [ -z "${default_if}" ]; then
          # If no non-utun default, take first one
          default_if=$(${timeout_cmd} netstat -rnf inet6 2>/dev/null | \
            awk '/^default/ {print $4; exit}')
        fi
        [ -n "${default_if}" ] && DEFAULT_INTERFACE_DETECTION="netstat"
      fi
      ;;

    *BSD)
      # FreeBSD, OpenBSD, NetBSD
      inet_flag=""
      [ "${ip_version}" = "-6" ] && inet_flag="-inet6"

      # route with -n to prevent DNS lookups
      default_if=$(${timeout_cmd} route -n get ${inet_flag} default 2>/dev/null | \
        awk '/interface:/ {print $2; exit}')
      [ -n "${default_if}" ] && DEFAULT_INTERFACE_DETECTION="route"

      # Fallback to netstat
      if [ -z "${default_if}" ]; then
        if [ "${ip_version}" = "-4" ]; then
          default_if=$(${timeout_cmd} netstat -rnf inet 2>/dev/null | \
            awk '/^default/ {print $4; exit}')
        else
          default_if=$(${timeout_cmd} netstat -rnf inet6 2>/dev/null | \
            awk '/^default/ {print $4; exit}')
        fi
        [ -n "${default_if}" ] && DEFAULT_INTERFACE_DETECTION="netstat"
      fi
      ;;
  esac

  # Get IP address from interface
  if [ -n "${default_if}" ] && [ "${default_if}" != "lo" ]; then
    # Skip if interface is down (Linux only)
    if [ "${KERNEL_NAME}" = "Linux" ] && [ -r "/sys/class/net/${default_if}/operstate" ]; then
      state=$(cat "/sys/class/net/${default_if}/operstate" 2>/dev/null)
      if [ "${state}" = "down" ]; then
        return
      fi
    fi

    if [ "${ip_version}" = "-4" ]; then
      # IPv4 handling
      # Try ip command first on Linux
      if [ "${KERNEL_NAME}" = "Linux" ] && command -v ip >/dev/null 2>&1; then
        # With -o flag, inet is field 3, IP is field 4
        ip_addr=$(${timeout_cmd} ip -o -4 addr show dev "${default_if}" 2>/dev/null | \
          awk '$3 == "inet" && $0 !~ /secondary/ {gsub(/\/.*/, "", $4); print $4; exit}')
      fi

      # Fallback to ifconfig (if available)
      if [ -z "${ip_addr}" ] && command -v ifconfig >/dev/null 2>&1; then
        ip_addr=$(${timeout_cmd} ifconfig "${default_if}" 2>/dev/null | \
          awk '/inet / && !/127.0.0.1/ {gsub(/addr:/, "", $2); print $2; exit}')
      fi

      # Validate IPv4
      if [ -n "${ip_addr}" ]; then
        if echo "${ip_addr}" | grep -qE '^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$' && \
           echo "${ip_addr}" | awk -F. '$1<=255 && $2<=255 && $3<=255 && $4<=255 {exit 0} {exit 1}'; then
          DEFAULT_INTERFACE_IP="${ip_addr}"
          DEFAULT_INTERFACE_NAME="${default_if}"
        fi
      fi
    else
      # IPv6 handling
      if [ "${KERNEL_NAME}" = "Linux" ] && command -v ip >/dev/null 2>&1; then
        # Get global scope IPv6, not link-local
        ip_addr=$(${timeout_cmd} ip -o -6 addr show dev "${default_if}" scope global 2>/dev/null | \
          awk '$3 == "inet6" && $0 !~ /deprecated/ {gsub(/\/.*/, "", $4); print $4; exit}')
      fi

      if [ -z "${ip_addr}" ] && command -v ifconfig >/dev/null 2>&1; then
        # Skip fe80:: (link-local) and ::1 (loopback)
        ip_addr=$(${timeout_cmd} ifconfig "${default_if}" 2>/dev/null | \
          awk '/inet6 / && !/fe80:/ && !/::1/ {sub(/%.*/, "", $2); print $2; exit}')
      fi

      # Basic IPv6 validation
      if [ -n "${ip_addr}" ] && echo "${ip_addr}" | grep -qE '^[0-9a-fA-F:]+$' && \
         echo "${ip_addr}" | grep -q ':'; then
        DEFAULT_INTERFACE_IP="${ip_addr}"
        DEFAULT_INTERFACE_NAME="${default_if}"
      fi
    fi
  fi
}

get_default_interface_ip -4

echo "NETDATA_CONTAINER_OS_NAME=${CONTAINER_NAME}"
echo "NETDATA_CONTAINER_OS_ID=${CONTAINER_ID}"
echo "NETDATA_CONTAINER_OS_ID_LIKE=${CONTAINER_ID_LIKE}"
echo "NETDATA_CONTAINER_OS_VERSION=${CONTAINER_VERSION}"
echo "NETDATA_CONTAINER_OS_VERSION_ID=${CONTAINER_VERSION_ID}"
echo "NETDATA_CONTAINER_OS_DETECTION=${CONTAINER_OS_DETECTION}"
echo "NETDATA_CONTAINER_IS_OFFICIAL_IMAGE=${CONTAINER_IS_OFFICIAL_IMAGE}"
echo "NETDATA_HOST_OS_NAME=${HOST_NAME}"
echo "NETDATA_HOST_OS_ID=${HOST_ID}"
echo "NETDATA_HOST_OS_ID_LIKE=${HOST_ID_LIKE}"
echo "NETDATA_HOST_OS_VERSION=${HOST_VERSION}"
echo "NETDATA_HOST_OS_VERSION_ID=${HOST_VERSION_ID}"
echo "NETDATA_HOST_OS_DETECTION=${HOST_OS_DETECTION}"
echo "NETDATA_HOST_IS_K8S_NODE=${HOST_IS_K8S_NODE}"
echo "NETDATA_SYSTEM_KERNEL_NAME=${KERNEL_NAME}"
echo "NETDATA_SYSTEM_KERNEL_VERSION=${KERNEL_VERSION}"
echo "NETDATA_SYSTEM_ARCHITECTURE=${ARCHITECTURE}"
echo "NETDATA_SYSTEM_VIRTUALIZATION=${VIRTUALIZATION}"
echo "NETDATA_SYSTEM_VIRT_DETECTION=${VIRT_DETECTION}"
echo "NETDATA_SYSTEM_CONTAINER=${CONTAINER}"
echo "NETDATA_SYSTEM_CONTAINER_DETECTION=${CONT_DETECTION}"
echo "NETDATA_SYSTEM_CPU_LOGICAL_CPU_COUNT=${LCPU_COUNT}"
echo "NETDATA_SYSTEM_CPU_VENDOR=${CPU_VENDOR}"
echo "NETDATA_SYSTEM_CPU_MODEL=${CPU_MODEL}"
echo "NETDATA_SYSTEM_CPU_FREQ=${CPU_FREQ}"
echo "NETDATA_SYSTEM_CPU_DETECTION=${CPU_INFO_SOURCE}"
echo "NETDATA_SYSTEM_TOTAL_RAM=${TOTAL_RAM}"
echo "NETDATA_SYSTEM_RAM_DETECTION=${RAM_DETECTION}"
echo "NETDATA_SYSTEM_TOTAL_DISK_SIZE=${DISK_SIZE}"
echo "NETDATA_SYSTEM_DISK_DETECTION=${DISK_DETECTION}"
echo "NETDATA_INSTANCE_CLOUD_TYPE=${CLOUD_TYPE}"
echo "NETDATA_INSTANCE_CLOUD_INSTANCE_TYPE=${CLOUD_INSTANCE_TYPE}"
echo "NETDATA_INSTANCE_CLOUD_INSTANCE_REGION=${CLOUD_INSTANCE_REGION}"
echo "NETDATA_SYSTEM_DEFAULT_INTERFACE_NAME=${DEFAULT_INTERFACE_NAME}"
echo "NETDATA_SYSTEM_DEFAULT_INTERFACE_IP=${DEFAULT_INTERFACE_IP}"
echo "NETDATA_SYSTEM_DEFAULT_INTERFACE_DETECTION=${DEFAULT_INTERFACE_DETECTION}"
