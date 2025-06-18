#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=./packaging/makeself/functions.sh
. "${NETDATA_MAKESELF_PATH}"/functions.sh "${@}" || exit 1

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Checking Go plugin CPU architecture" || true

check_files="${NETDATA_INSTALL_PATH}/bin/netdata ${NETDATA_INSTALL_PATH}/usr/libexec/netdata/plugins.d/go.d.plugin"

case "${BUILDARCH}" in
  aarch64)
    ELF_MACHINE="AArch64"
    ELF_CLASS="ELF64"
    ;;
  armv6l | armv7l)
    ELF_MACHINE="ARM"
    ELF_CLASS="ELF32"
    ;;
  x86_64)
    ELF_MACHINE="X86-64"
    ELF_CLASS="ELF64"
    ;;
  *)
    echo "Buildarch is not recognized for architecture check."
    exit 1
    ;;
esac

for f in ${check_files}; do
  if [ ! -f "${f}" ]; then
    echo "File ${f} not found, skipping check"
    continue
  fi

  # Get both the machine type and the class (32-bit vs 64-bit)
  elf_info=$(readelf -h "${f}")
  file_machine=$(echo "${elf_info}" | grep 'Machine:' | awk -F: '{print $2}' | xargs)
  file_class=$(echo "${elf_info}" | grep 'Class:' | awk -F: '{print $2}' | xargs)

  echo "Checking ${f}:"
  echo "  Expected: Class=${ELF_CLASS}, Machine contains '${ELF_MACHINE}'"
  echo "  Found:    Class=${file_class}, Machine='${file_machine}'"

  if [ "${file_class}" != "${ELF_CLASS}" ]; then
    echo "ERROR: ${f} has wrong ELF class (${file_class} instead of ${ELF_CLASS})"
    echo "This indicates a 32-bit/64-bit mismatch!"
    exit 1
  fi

  if ! echo "${file_machine}" | grep -q "${ELF_MACHINE}"; then
    echo "ERROR: ${f} was built for the wrong architecture (${file_machine} does not contain ${ELF_MACHINE})"
    exit 1
  fi
done

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
