#!/usr/bin/env bash

get_kernel_version() {
  r="$(uname -r | cut -f 1 -d '-')"

  read -r -a p <<< "$(echo "${r}" | tr '.' ' ')"

  printf "%03d%03d%03d" "${p[0]}" "${p[1]}" "${p[2]}"
}

get_rh_version() {
  if [ ! -f /etc/redhat-release ]; then
    printf "000000000"
    return
  fi

  r="$(cut -f 4 -d ' ' < /etc/redhat-release)"

  read -r -a p <<< "$(echo "${r}" | tr '.' ' ')"

  printf "%03d%03d%03d" "${p[0]}" "${p[1]}" "${p[2]}"
}

if [ "$(uname -s)" != "Linux" ]; then
  echo >&2 "This does not appear to be a Linux system."
  exit 1
fi

KERNEL_VERSION="$(uname -r)"

if [ "$(get_kernel_version)" -lt 004014000 ] && [ "$(get_rh_version)" -lt 0070061810 ]; then
  echo >&2 "WARNING: Your kernel appears to be older than 4.11 or you are using RH version older than 7.6.1810. This may still work in some cases, but probably won't."
fi

CONFIG_PATH=""
MODULE_LOADED=""

if modprobe configs 2> /dev/null; then
  MODULE_LOADED=1
fi

if [ -r /proc/config.gz ]; then
  CONFIG_PATH="/proc/config.gz"
elif [ -r "/lib/modules/${KERNEL_VERSION}/source/.config" ]; then
  CONFIG_PATH="/lib/modules/${KERNEL_VERSION}/source/.config"
elif [ -r "/lib/modules/${KERNEL_VERSION}.x86_64/source/.config" ]; then
  CONFIG_PATH="/lib/modules/${KERNEL_VERSION}.x86_64/source/.config"
elif [ -n "$(find /boot -name "config-${KERNEL_VERSION}*")" ]; then
  CONFIG_PATH="$(find /boot -name "config-${KERNEL_VERSION}*" | head -n 1)"
fi

if [ -n "${CONFIG_PATH}" ]; then
  GREP='grep'
  CAT='cat'

  if echo "${CONFIG_PATH}" | grep -q '.gz'; then
    CAT='zcat'
  fi

  REQUIRED_CONFIG="KPROBES KPROBES_ON_FTRACE HAVE_KPROBES BPF BPF_SYSCALL BPF_JIT"

  for required_config in ${REQUIRED_CONFIG}; do
    # Fix issue https://github.com/netdata/netdata/issues/14668
    # if ! "${GREP}" -q "CONFIG_${required_config}=y" "${CONFIG_PATH}"; then
    if ! { "${CAT}" "${CONFIG_PATH}" | "${GREP}" -q "CONFIG_${required_config}=y" >&2 >/dev/null; } ;then
      echo >&2 " Missing Kernel Config: ${required_config}"
      exit 1
    fi
  done
fi

if [ -n "${MODULE_LOADED}" ]; then
  modprobe -r configs 2> /dev/null || true # Ignore failures from CONFIGS being builtin
fi

exit 0
