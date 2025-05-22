#!/usr/bin/env bash

if [ ! -d "config" ]; then
  echo "Please create a config directory with the necessary configuration files."
  exit 1
fi

mkdir -p reports/clients

docker run -it --rm \
  -v ${PWD}/config:/config \
  -v ${PWD}/reports:/reports \
  -p 9001:9001 \
  crossbario/autobahn-testsuite \
  wstest -m fuzzingclient -s /config/fuzzingclient.json
