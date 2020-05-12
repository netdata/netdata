#!/bin/bash

# buildhtml.sh

set -e

# Assumes that the script is executed either from the htmldoc folder (by netlify), or from the root repo dir (as originally intended)
currentdir=$(pwd | awk -F '/' '{print $NF}')
echo "$currentdir"
if [ "$currentdir" = "generator" ]; then
	cd ../..
fi
GENERATOR_DIR="docs/generator"
SRC_DIR="${GENERATOR_DIR}/src"
# Fetch go.d.plugin docs
GO_D_DIR="collectors/go.d.plugin"
rm -rf ${GO_D_DIR}
git clone https://github.com/netdata/go.d.plugin.git ${GO_D_DIR}
find "${GO_D_DIR}" -maxdepth 1 -mindepth 1 -type d ! -name modules -exec rm -rf '{}' \;

# Copy all Netdata .md files to docs/generator/src. Exclude htmldoc itself and also the directory node_modules generatord by Netlify
echo "Copying files"
rm -rf ${SRC_DIR}
mkdir ${SRC_DIR}
find . -type d \( -path ./${GENERATOR_DIR} -o -path ./node_modules \) -prune -o -name "*.md" -exec cp -pr --parents '{}' ./${SRC_DIR}/ ';'

# Remove specific files that don't belong in the documentation
declare -a EXCLUDE_LIST=(
	"HISTORICAL_CHANGELOG.md"
	"contrib/sles11/README.md"
)

for f in "${EXCLUDE_LIST[@]}"; do
	rm "${SRC_DIR}/$f"
done

echo "Preparing directories"
MKDOCS_DIR="doc"
DOCS_DIR=${GENERATOR_DIR}/${MKDOCS_DIR}
rm -rf ${DOCS_DIR}

echo "Preparing source"
cp -r ${SRC_DIR} ${DOCS_DIR}

echo "Looking for broken links"
# Fix links (recursively, all types, executing replacements)
${GENERATOR_DIR}/checklinks.sh -rax

# Remove cloned projects and temp directories
rm -rf ${GO_D_DIR} ${LOC_DIR} ${DOCS_DIR} ${SRC_DIR}

echo "Finished"
