#!/bin/bash

set -e

pkgdir="${1}"
keyid="${2}"

echo "::group::Installing Dependencies"
sudo apt-get update
sudo apt-get upgrade -y
sudo apt-get install -y debsigs
echo "::endgroup::"

echo "::group::Signing packages"
debsigs --sign=origin --default-key="${keyid}" "${pkgdir}"/*.{,d}deb
echo "::endgroup::"
