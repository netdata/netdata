#!/bin/bash

set -e

pkgdir="${1}"
keyid="${2}"

echo "::group::Installing Dependencies"
apt update
apt upgrade -y
apt install -y debsigs
echo "::endgroup::"

echo "::group::Signing packages"
debsigs --sign=origin --default-keyid="${keyid}" "${pkgdir}"/*.{,d}deb
echo "::endgroup::"
