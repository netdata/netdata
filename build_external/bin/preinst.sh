#!/usr/bin/env bash

Distro=$1
Version=$2
OutBase="$(cd "$(dirname "$0")" && cd .. && pwd)"
Dockerfile=$OutBase/"$Distro"_"$Version"_preinst.Dockerfile
ImageName="$Distro"_"$Version"_preinst

if [ ! -e "$Dockerfile" ]
then
  echo "Can't build preinstaller from $Dockerfile"
  return -1
fi

docker build -t $ImageName -f $Dockerfile $OutBase/empty

