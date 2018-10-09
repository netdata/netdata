#!/bin/bash

set -e

docker build -t dev-image -f ".travis/images/Dockerfile.$1" .

docker run -it -w /code dev-image ./netdata-installer.sh --dont-wait --dont-start-it --install /tmp
