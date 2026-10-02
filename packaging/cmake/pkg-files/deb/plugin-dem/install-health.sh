#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later

# Arguments are installation paths and account names; keep them out of logs.
run_dem_install() {
  printf >&2 '+ %s [arguments redacted]\n' "$1"
  "$@"
  dem_install_status=$?
  if [ "$dem_install_status" -ne 0 ]; then
    printf >&2 'DEM installation: %s failed in %s (status %s)\n' "$1" "$PWD" "$dem_install_status"
  fi
  return "$dem_install_status"
}

install_dem_health() {
  if [ "$#" -ne 4 ]; then
    printf >&2 'Usage: dem-install-health.sh LIB_DIR CONFIG_DIR USER GROUP\n'
    return 2
  fi
  dem_state_dir="$1/dem"
  dem_health_dir="$dem_state_dir/health.d"
  dem_config_dir="$2/health.d"
  dem_health_link="$dem_config_dir/dem-generated"
  dem_owner="$3:$4"

  for dem_dir in "$dem_state_dir" "$dem_health_dir" "$dem_config_dir"; do
    if [ -L "$dem_dir" ] || { [ -e "$dem_dir" ] && [ ! -d "$dem_dir" ]; }; then
      printf >&2 'WARNING: DEM health setup found a conflicting directory entry; preserving it.\n'
      return 0
    fi
  done
  if [ -L "$dem_health_link" ]; then
    if [ "$(readlink "$dem_health_link")" != "$dem_health_dir" ]; then
      printf >&2 'WARNING: DEM health link has a different target; preserving it.\n'
      return 0
    fi
  elif [ -e "$dem_health_link" ]; then
    printf >&2 'WARNING: DEM health link path already exists; preserving it.\n'
    return 0
  fi

  run_dem_install mkdir -p "$dem_health_dir" "$dem_config_dir" || return $?
  if [ "$(id -u)" -eq 0 ]; then
    run_dem_install chown "$dem_owner" "$dem_state_dir" "$dem_health_dir" || return $?
  fi
  run_dem_install chmod 0750 "$dem_state_dir" "$dem_health_dir" || return $?
  if [ ! -L "$dem_health_link" ]; then
    run_dem_install ln -s "$dem_health_dir" "$dem_health_link" || return $?
  fi
}

install_dem_health "$@"
