#!/usr/bin/env bash

# Basic setup, check we have a legal Distro / Version combo
Distro=$1
Version=$2
Repo=$(cd $(dirname $0) && cd ../.. && pwd)
Base=$Repo/build_external
Arch=$(uname)

TagName=clean_install_"$Distro"_"$Version":latest
Dockerfile=$Base/clean_install_"$Distro"_"$Version".Dockerfile

if [ ! -e "$Dockerfile" ]; then
    echo "Can't run installer on $Distro/$Version - $Dockerfile not found"
    exit -1
fi

if [ "$Arch" == "Linux" ]; then
    sudo docker build -f "$Dockerfile" "$Repo" -t "$TagName"
elif [ "$Arch" == "Darwin" ]; then
    docker build -f "$Dockerfile" "$Repo" -t "$TagName"
fi
