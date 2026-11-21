#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

set -Eeuo pipefail
trap 'status=$?; printf "DEM image check failed in %s (status %s)\n" "$PWD" "$status" >&2; exit "$status"' ERR

image="${1:?Expected a locally built DEM image}"
root=/opt/netdata-dem-runtime
plugin=/usr/libexec/netdata/plugins.d/dem.plugin
helper=packaging/installer/netdata-dem-runtime.sh
deployment="docker-dem-ci-${GITHUB_RUN_ID:?}-${GITHUB_RUN_ATTEMPT:?}-$(uname -m)"
qualification="$deployment-qualification"
agent="$deployment-agent"
label="netdata.ci.dem=$deployment"
registered=no
fixture_dir=

cleanup() {
    local status=$? failed=0 containers name
    trap - EXIT ERR
    if containers="$(docker ps --all --filter "label=$label" --format '{{.Names}}')"; then
        while IFS= read -r name; do
            case "$name" in
                "$agent")
                    if [ "$status" -ne 0 ]; then docker logs "$name" >&2 || :; fi
                    docker rm -fv "$name" || failed=1 ;;
                "$qualification") docker rm -fv "$name" || failed=1 ;;
                '') ;;
                *) printf 'Unexpected container with test label: %s\n' "$name" >&2; failed=1 ;;
            esac
        done <<< "$containers"
    else
        failed=1
    fi
    if [ "$failed" -eq 0 ] && [ "$registered" = yes ]; then
        sudo sh "$helper" policy-remove --deployment "$deployment" --deployments-stopped || failed=1
    fi
    if [ -n "$fixture_dir" ]; then rm -rf -- "$fixture_dir" || failed=1; fi
    if [ "$status" -eq 0 ]; then status=$failed; fi
    exit "$status"
}
trap cleanup EXIT

# These invocations contain only local image names, paths and a CI deployment ID.
set -x
docker run --pull=never --rm --network none --read-only --user netdata --entrypoint /bin/sh "$image" -ec '
    root=$1
    test "$(stat -c %u:%g "$root")" = "$(id -u):$(id -g)"
    test -r "$root"
    test -x "$root"
    case "$(stat -c %a "$root")" in *00) ;; *) exit 1 ;; esac
    test -x "$2"
    test -r /usr/lib/netdata/conf.d/dem.conf
    test -r /usr/libexec/netdata/netdata-dem-runtime.sh
    stock=/usr/share/netdata/topology-ip-intel
    for file in README.md topology-ip-asn.mmdb topology-ip-geo.mmdb topology-ip-intel.json; do
        test -r "$stock/$file"
        test -s "$stock/$file"
    done
    test ! -e /var/lib/netdata/dem/runtime
' sh "$root" "$plugin"

docker run --pull=never --rm --network none --read-only --user netdata \
    --entrypoint "$plugin" "$image" runtime preload-validate |
    jq -e --arg root "$root" --arg version "${NETDATA_BUILD_VERSION:?}" \
        '.state == "installed" and .mode == "preloaded" and .preload_path == $root and .runtime.version == $version'

registered=yes
sudo sh "$helper" policy-prepare --deployment "$deployment" --runtime-root "$root"

# The container retains Docker's default seccomp and runs the browser sandbox.
# Use the documented Agent capabilities and AppArmor setting on this runner.
docker run --pull=never --name "$qualification" --label "$label" --network none --pid host \
    --cap-add SYS_PTRACE --cap-add SYS_ADMIN --security-opt apparmor=unconfined \
    --read-only --tmpfs /tmp:mode=1777 --user netdata --entrypoint "$plugin" "$image" runtime qualify |
    jq -e --arg version "$NETDATA_BUILD_VERSION" '.state == "qualified" and .runtime.version == $version and
        .qualification.execution_contract and .qualification.sandboxed_local_journey and
        .qualification.sandboxed_local_lighthouse and .qualification.process_tree_drained and
        .runtime == .qualification.runtime'

# Run the normal entrypoint without network access. The fixture leaves geoip_db
# unset and sends a known lookup address through the trusted loopback proxy path.
fixture_dir="$(mktemp -d)"
chmod 0755 "$fixture_dir"
cat > "$fixture_dir/receiver.conf" <<'YAML'
jobs:
  - name: receiver
    listen: 127.0.0.1:19938
    public_url: http://127.0.0.1:19938
    trusted_proxies: [127.0.0.1/32]
YAML
cat > "$fixture_dir/rum.conf" <<'YAML'
jobs:
  - name: image-smoke
    allowed_origins: [https://fixture.example]
    update_every: 1
    capture:
      geolocation: country
YAML
chmod 0644 "$fixture_dir/"*.conf
docker run --pull=never --detach --name "$agent" --label "$label" --network none \
    --mount "type=bind,src=$fixture_dir,dst=/etc/netdata/dem,readonly" \
    --mount type=bind,src=/etc/machine-id,dst=/host/etc/machine-id,readonly \
    -e DISABLE_TELEMETRY=1 "$image"

agent_curl() {
    docker exec "$agent" curl --noproxy '*' --silent --show-error --max-time 3 "$@"
}

agent_curl --fail --retry 15 --retry-delay 1 --retry-connrefused http://127.0.0.1:19938/rum/ping
report='{"meta":{"page":{"id":"document-1","url":"https://fixture.example/"},"session":{"id":"image-smoke"},"browser":{"name":"Chrome"}},"events":[{"name":"document_activated","attributes":{"observation_id":"document-1","observation_sequence":"1"}}]}'
for ((attempt = 1; attempt <= 10; attempt++)); do
    http_status="$(agent_curl --output /dev/stderr --write-out '%{http_code}' \
        -H 'Content-Type: application/json' -H 'Origin: https://fixture.example' \
        -H 'User-Agent: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36' \
        -H 'X-Forwarded-For: 8.8.8.8' --data-binary "$report" http://127.0.0.1:19938/rum/image-smoke/collect)"
    case "$http_status" in
        202) break ;;
        404|503) sleep 1 ;;
        *) printf 'Unexpected RUM collect status: %s\n' "$http_status" >&2; exit 1 ;;
    esac
done
if [ "$http_status" != 202 ]; then
    printf 'RUM report was not accepted: HTTP %s\n' "$http_status" >&2
    exit 1
fi

for ((attempt = 1; attempt <= 15; attempt++)); do
    chart="$(agent_curl --fail http://127.0.0.1:19999/api/v1/charts |
        jq -r '[.charts | to_entries[] | select(.value.context == "rum.country_document_views" and
            .value.chart_labels.country == "US") | .key] | first // empty')"
    if [ -n "$chart" ]; then
        samples="$(agent_curl --fail --get --data-urlencode "chart=$chart" \
            --data 'after=-30&points=30&format=json' http://127.0.0.1:19999/api/v1/data)"
        if jq -e 'any(.data[]?; any(.[1:][]; type == "number" and . > 0))' <<< "$samples"; then
            printf 'RUM recorded document views for country US in %s\n' "$chart"
            exit 0
        fi
    fi
    sleep 1
done
echo 'RUM did not publish a positive US country document-view sample' >&2
exit 1
