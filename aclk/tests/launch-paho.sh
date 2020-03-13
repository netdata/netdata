#!/usr/bin/env bash

docker build -f paho.Dockerfile . --build-arg "HOST_HOSTNAME=$(ping -c1 "$(hostname).local" | head -n1 | grep -o '[0-9]*\.[0-9]*\.[0-9]*\.[0-9]*')" -t paho-client
docker run -it paho-client
