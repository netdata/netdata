#!/usr/bin/env bash

Distro=$1
Version=$2
OutBase="$(cd "$(dirname "$0")" && cd .. && pwd)"
InBase=/opt/netdata
Config="$Distro"_"$Version"
ImageName="$Config"_dev
#Dockerfile=$OutBase/"$Distro"_"$Version"_preinst.Dockerfile
AbsSrc="$(readlink "$OutBase/netdata" || echo "$OutBase/netdata")"

if [ "$Config" != "$(cat "$AbsSrc/build_external/current")" ]
then
  echo "The installed version does not match: $(cat "$AbsSrc/build_external/current")" 
  exit 1
fi

docker rm "$ImageName" 2>/dev/null

docker run -it --mount "type=bind,source=$AbsSrc,target=$InBase/source" \
           --name "$ImageName" \
           -w "$InBase/source" "$ImageName" \
           "$InBase/install/netdata/usr/sbin/netdata" -D
