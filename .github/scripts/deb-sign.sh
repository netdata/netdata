#!/bin/bash

set -e

pkgdir="${1}"
keyid="${2}"

echo "::group::Installing Dependencies"
# Only install what signing needs. A full `apt-get upgrade` pulls in unrelated
# runner packages, including the firefox snap transition stub, which makes
# signing depend on the Snap Store being reachable.
sudo apt-get update
sudo apt-get install -y debsigs
echo "::endgroup::"

echo "::group::Signing packages"
debsigs --sign=origin --default-key="${keyid}" "${pkgdir}"/*.{,d}deb
echo "::endgroup::"
