#!/bin/bash

# buildhtml.sh

# Builds the html static site, using mkdocs
# Assumes that the script is executed from the root netdata folder, by calling htmldoc/buildhtml.sh

# Copy all netdata .md files to htmldoc/src 
rm -rf htmldoc/src
find . -path ./htmldoc -prune -o -name "*.md" -print | cpio -pd htmldoc/src

# Modify the first line of the main README.md, to enable proper static html generation 
sed -i '0,/# netdata /s//# Introducing NetData\n\n/' htmldoc/src/README.md

# Generate mkdocs.yaml
htmldoc/buildyaml.sh > htmldoc/mkdocs.yml

# Build html docs
mkdocs build --config-file=htmldoc/mkdocs.yml


