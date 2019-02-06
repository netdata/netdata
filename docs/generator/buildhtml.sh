#!/bin/bash

# buildhtml.sh

# Builds the html static site, using mkdocs

set -e

# Assumes that the script is executed either from the htmldoc folder (by netlify), or from the root repo dir (as originally intended)
currentdir=$(pwd | awk -F '/' '{print $NF}')
echo "$currentdir"
if [ "$currentdir" = "generator" ]; then
	cd ../..
fi
GENERATOR_DIR="docs/generator"

# Fetch go.d.plugin docs
rm -rf ./collectors/go.d.plugin
git clone https://github.com/netdata/go.d.plugin.git ./collectors/go.d.plugin

# Copy all netdata .md files to docs/generator/src. Exclude htmldoc itself and also the directory node_modules generatord by Netlify
echo "Copying files"
rm -rf ${GENERATOR_DIR}/src
find . -type d \( -path ./${GENERATOR_DIR} -o -path ./node_modules \) -prune -o -name "*.md" -print | cpio -pd ${GENERATOR_DIR}/src

# Copy netdata html resources
cp -a ./${GENERATOR_DIR}/custom ./${GENERATOR_DIR}/src/



# Modify the first line of the main README.md, to enable proper static html generation
echo "Modifying README header"
sed -i -e '0,/# netdata /s//# Introduction\n\n/' ${GENERATOR_DIR}/src/README.md

# Remove all GA tracking code
find ${GENERATOR_DIR}/src -name "*.md" -print0 | xargs -0 sed -i -e 's/\[!\[analytics.*UA-64295674-3)\]()//g'

# Remove specific files that don't belong in the documentation
declare -a EXCLUDE_LIST=(
	"HISTORICAL_CHANGELOG.md"
	"contrib/sles11/README.md"
	"packaging/maintainers/README.md"
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

echo "Calling mkdocs"

# Build html docs
mkdocs build --config-file=${GENERATOR_DIR}/mkdocs.yml

# Fix edit buttons for the markdowns that are not on the main netdata repo
find ${GENERATOR_DIR}/build/collectors/go.d.plugin -name "*.html" -print0 | xargs -0 sed -i -e 's/https:\/\/github.com\/netdata\/netdata\/blob\/master\/collectors\/go.d.plugin/https:\/\/github.com\/netdata\/go.d.plugin\/blob\/master/g'

# Remove the cloned go.d.plugin project
rm -rf ./collectors/go.d.plugin

echo "Finished"
