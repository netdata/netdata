#!/bin/bash

# buildhtml.sh

# Builds the html static site, using mkdocs
# Assumes that the script is executed from the root netdata folder, by calling htmldoc/buildhtml.sh

# Copy all netdata .md files to htmldoc/src 
rm -rf htmldoc/src
find . -path ./htmldoc -prune -o -name "*.md" -print | cpio -pd htmldoc/src

# Generate mkdocs.yaml
htmldoc/buildyaml.sh > htmldoc/mkdocs.yml

# Build html docs
mkdocs build --config-file=htmldoc/mkdocs.yml


