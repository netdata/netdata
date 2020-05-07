#!/bin/sh

VERSION="$(git describe)"
echo "$VERSION" > packaging/version
git commit -a -m "[netdata nightly] $VERSION"
