#!/bin/bash

# Builds the html static site, using mkdocs
# Assumes that 
# Copy all # netdata .md files to source/# netdata 
rm -rf doc/src/git
find . -path doc -prune -o -name "*.md" | cpio -pd doc/src/git
mkdocs build --config-file=doc/mkdocs.yml


