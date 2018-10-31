#!/bin/bash

# buildhtml.sh

# Builds the html static site, using mkdocs
# Assumes that the script is executed from the root netdata folder, by calling htmldoc/buildhtml.sh

# Copy all # netdata .md files to source/# netdata 
rm -rf htmldoc/src
find . -path htmldoc -prune -o -name "*.md" | cpio -pd htmldoc/src

# Build html docs
mkdocs build --config-file=htmldoc/mkdocs.yml


