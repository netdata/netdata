# shellcheck shell=bash
# no need for shebang - this file is loaded from charts.d.plugin
# SPDX-License-Identifier: GPL-3.0-or-later

# netdata
# real-time performance and health monitoring, done right!
# (C) 2020 Sander Klein <netdata@roedie.nl>
#

# _update_every is a special variable - it holds the number of seconds
# between the calls of the _update() function

# how frequently to collect ZFS dataset data
zfsdstat_update_every=1

# the priority of zfsdstat related to other charts
# 2900 should place it right beneath the ZFS filesystem charts
zfsdstat_priority=2900

zfsdstat_kstat="/proc/spl/kstat/zfs"
declare -A zfsdstat_datasets

zfsdstat_find_all_datasets() {
  local i objs
  objs="$(find $zfsdstat_kstat -type f -name "objset-*")"
  for i in $objs; do
    zfsdstat_datasets["$i"]="$(awk -v RS="" '{print $13}' "$i")"
  done

  if [ "${#zfsdstat_datasets[@]}" -eq 0 ]; then return 1; fi

  return 0
}

zfsdstat_check() {
  # Check if ZFS on Linux is active
  if [ ! -d "$zfsdstat_kstat" ]; then return 1; fi

  # Try to find *any* dataset
  # ZFS versions < 0.8.0 do not have the needed /proc entries
  zfsdstat_find_all_datasets
  if [ "$?" -eq 1 ]; then
    return 1
  fi

  return 0
}

zfsdstat_create() {
  local i dataset

  for i in "${!zfsdstat_datasets[@]}"; do
    dataset="${zfsdstat_datasets[$i]//\//_}"

    cat << EOF
CHART zfsdstat.${dataset}_iops '' "ZFS Dataset IOPS" "operations/s" "${zfsdstat_datasets[$i]}" 'zfsdstat.iops' 'area' $((zfsdstat_priority + 1)) $zfsdstat_update_every
DIMENSION reads '' incremental 1 1
DIMENSION writes '' incremental -1 1

CHART zfsdstat.${dataset}_bandwidth '' "ZFS Dataset Bandwidth" "B/s" "${zfsdstat_datasets[$i]}" 'zfsdstat.bandwidth' 'area' $((zfsdstat_priority + 2)) $zfsdstat_update_every
DIMENSION nreads 'reads' incremental 1 1
DIMENSION nwritten 'writes' incremental -1 1

CHART zfsdstat.${dataset}_unlinks '' "ZFS Dataset Unlinks" "operations/s" "${zfsdstat_datasets[$i]}" 'zfsdstat.unlinks' 'line' $((zfsdstat_priority + 3)) $zfsdstat_update_every
DIMENSION nunlinks 'unlinks' incremental 1 1
DIMENSION nunlinked 'unlinked' incremental 1 1
EOF

  done

  return 0
}

zfsdstat_update() {
  local i reads writes nreads nwritten nunlinks nunlinked dataset
  for i in "${!zfsdstat_datasets[@]}"; do
    dataset=${zfsdstat_datasets[$i]//\//_}
    read -r reads writes nreads nwritten nunlinks nunlinked <<< "$(awk -v RS="" '{print $22" "$16" "$25" "$19" "$28" "$31}' "$i")"

    echo BEGIN zfsdstat."${dataset}"_iops "$1"
    echo SET reads "$reads"
    echo SET writes "$writes"
    echo END
    echo BEGIN zfsdstat."${dataset}"_bandwidth "$1"
    echo SET nreads "$nreads"
    echo SET nwritten "$nwritten"
    echo END
    echo BEGIN zfsdstat."${dataset}"_unlinks "$1"
    echo SET nunlinks "$nunlinks"
    echo SET nunlinked "$nunlinked"
    echo END

  done

  return 0
}
