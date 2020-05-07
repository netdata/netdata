#!/bin/sh

cid=

create_container() {
  progress "  Creating Container"
  cid="$(docker run -d -p "$HOST_PORT:$CONTAINER_PORT" --name "$CONTAINER_NAME" netdata/netdata:test)"
  sleep 5
}

wait_container() {
  progress "  Waiting for Container"
  (
    if ! timeout "$CONTAINER_WAIT_TIMEOUT" wait_for_tcp.sh "$HOST_IP" "$HOST_PORT"; then
      docker logs "$cid"
      fail "Container did not come up in time!"
    fi
  ) >&2
}

setup() {
  for setup_step in create_container wait_container; do
    if ! run "$setup_step"; then
      fail "Setup failed"
    fi
  done
}

cleanup() {
  if docker ps -a -q --no-trunc | grep -q "$cid"; then
    docker rm -f "$cid" > /dev/null
  fi
}

shutdown_container() {
  progress "  Shutting down Container"
  docker stop "$cid" > /dev/null
}

cleanup_container() {
  progress "  Cleaning up Container"
  if docker ps -a -q --no-trunc | grep -q "$cid"; then
    docker rm -f "$cid" > /dev/null
  fi
}

teardown() {
  for teardown_step in shutdown_container cleanup_container; do
    if ! run "$teardown_step"; then
      fail "Teardown failed"
    fi
  done
}
