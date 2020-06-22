#!/bin/sh

VERSION="$(git describe)"
echo "$VERSION" > packaging/version
git add -A
git ci -m "[netdata nightly] $VERSION"
