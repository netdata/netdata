#!/bin/bash

# buildhtml.sh

# Builds the html static site, using mkdocs
# Assumes that the script is executed either from the htmldoc folder (by netlify), or from the root repo dir (as originally intended)
currentdir=$(pwd | awk -F '/' '{print $NF}')
echo "$currentdir"
if [ "$currentdir" = "generator" ]; then
	cd ../..
fi
GENERATOR_DIR="docs/generator"

# Copy all netdata .md files to docs/generator/src. Exclude htmldoc itself and also the directory node_modules generatord by Netlify
echo "Copying files"
rm -rf ${GENERATOR_DIR}/src
find . -type d \( -path ./${GENERATOR_DIR} -o -path ./node_modules \) -prune -o -name "*.md" -print | cpio -pd ${GENERATOR_DIR}/src

# Modify the first line of the main README.md, to enable proper static html generation
sed -i '0,/# netdata /s//# Introduction\n\n/' ${GENERATOR_DIR}/src/README.md

# Remove specific files that don't belong in the documentation
declare -a EXCLUDE_LIST=(
	"HISTORICAL_CHANGELOG.md"
)
for f in "${EXCLUDE_LIST[@]}"; do
	rm "${GENERATOR_DIR}/src/$f"
done

echo "Creating mkdocs.yaml"

# Generate mkdocs.yaml
${GENERATOR_DIR}/buildyaml.sh >${GENERATOR_DIR}/mkdocs.yml

echo "Fixing links"

# Fix links (recursively, all types, executing replacements)
${GENERATOR_DIR}/checklinks.sh -rax
if [ $? -eq 1 ]; then exit 1; fi

echo "Calling mkdocs"

# Build html docs
mkdocs build --config-file=${GENERATOR_DIR}/mkdocs.yml

echo "Finished"
