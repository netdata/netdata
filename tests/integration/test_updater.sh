#!/bin/sh

# shellcheck disable=SC2034
description="Test that the updater works"

run_test() {
  (
    set -ex

    old_cli_version="$($EXEC_CMD /usr/sbin/netdata -v | awk '{ print $2 }')"
    old_api_version="$($CURL_CMD "$BASEURL/api/v1/info" | jq -r '.version')"
    printf >&2 "old_cli_version: %s\n" "$old_cli_version"
    printf >&2 "old_api_version: %s\n" "$old_api_version"
    assert_equals "$old_cli_version" "$old_api_version"

    # Update the container's Netdata
    # XXX: We use `docker exec` directly here and not `$EXEC_CMD` because
    #      we need to pass in some custom environment variables.
    docker exec \
      -e NETDATA_NIGHTLIES_BASEURL="$NETDATA_NIGHTLIES_BASEURL" \
      "$CONTAINER_NAME" \
      /usr/libexec/netdata/netdata-updater.sh

    new_cli_version="$($EXEC_CMD /usr/sbin/netdata -v)"
    new_api_version="$($CURL_CMD "$BASEURL/api/v1/info" | jq -r '.version')"
    printf >&2 "new_cli_version: %s\n" "$new_cli_version"
    printf >&2 "new_api_version: %s\n" "$new_api_version"
    assert_not_equals "$old_cli_version" "$new_cli_version"
    assert_not_equals "$old_api_version" "$new_api_version"
  ) >&2
}
