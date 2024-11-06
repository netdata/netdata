#!/bin/sh
#
set -e
# shellcheck source=.github/scripts/functions.sh
. "$(dirname "$0")/functions.sh"

check_successful_update() {
  progress "Check netdata version after update"
  (
    netdata_version=$(netdata -v | awk '{print $2}')
    updater_version=$(cat packaging/version)
    echo "Version: netdata '$netdata_version', updater '$updater_version'"
    if [ "$netdata_version" = "$updater_version" ]; then
      echo "Update successful!"
    else
      exit 1
    fi
  ) >&2
}

check_autoupdate_enabled() {
  progress "Check autoupdate still enabled after update"
  (
    if [ -f /etc/periodic/daily/netdata-updater ] || [ -f /etc/cron.daily/netdata-updater ]; then
      echo "Update successful!"
    else
      exit 1
    fi
  ) >&2
}

steps="check_successful_update check_autoupdate_enabled"

_main() {
  for step in $steps; do
    if ! run "$step"; then
      if [ -t 1 ]; then
        debug
      else
        fail "Build failed"
      fi
    fi
  done

  echo "All Done!"
}

if [ -n "$0" ] && [ x"$0" != x"-bash" ]; then
  _main "$@"
fi
