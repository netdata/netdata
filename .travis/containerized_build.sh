#!/bin/bash

set -e

docker run -it -w /code "netdata/os-test:$1" ./netdata-installer.sh --dont-wait --dont-start-it --install /tmp
