#!/usr/bin/env bash

Project="$1"
BuildBase=$(cd $(dirname $0) && cd .. && pwd)
ProjectBase="$BuildBase/projects/$Project"

eval $(cat $BuildBase/.env)
docker-compose -f "$ProjectBase/docker-compose.yml" up


