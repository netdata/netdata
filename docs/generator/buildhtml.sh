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
SRC_DIR="${GENERATOR_DIR}/src"
# Fetch go.d.plugin docs
GO_D_DIR="collectors/go.d.plugin"
rm -rf ${GO_D_DIR}
git clone https://github.com/netdata/go.d.plugin.git ${GO_D_DIR}

# Copy all Netdata .md files to docs/generator/src. Exclude htmldoc itself and also the directory node_modules generatord by Netlify
echo "Copying files"
rm -rf ${SRC_DIR}
mkdir ${SRC_DIR}
find . -type d \( -path ./${GENERATOR_DIR} -o -path ./node_modules \) -prune -o -name "*.md" -exec cp -prv --parents '{}' ./${SRC_DIR}/ ';'

# Copy Netdata html resources
cp -a ./${GENERATOR_DIR}/custom ./${SRC_DIR}/

# Modify the first line of the main README.md, to enable proper static html generation
echo "Modifying README header"
sed -i -e '0,/# Netdata /s//# Netdata Documentation\n\n/' ${SRC_DIR}/README.md

# Remove all GA tracking code
find ${SRC_DIR} -name "*.md" -print0 | xargs -0 sed -i -e 's/\[!\[analytics.*UA-64295674-3)\]()//g'

# Remove specific files that don't belong in the documentation
declare -a EXCLUDE_LIST=(
	"HISTORICAL_CHANGELOG.md"
	"contrib/sles11/README.md"
)

for f in "${EXCLUDE_LIST[@]}"; do
	rm "${SRC_DIR}/$f"
done

echo "Fetching localization project"
LOC_DIR=${GENERATOR_DIR}/localization
rm -rf ${LOC_DIR}
git clone https://github.com/netdata/localization.git ${LOC_DIR}

echo "Preparing directories"
MKDOCS_CONFIG_FILE="${GENERATOR_DIR}/mkdocs.yml"
MKDOCS_DIR="doc"
DOCS_DIR=${GENERATOR_DIR}/${MKDOCS_DIR}
rm -rf ${DOCS_DIR}

prep_html() {
	lang="${1}"
	echo "Creating ${lang} mkdocs.yaml"

	if [ "${lang}" == "en" ] ; then
		SITE_DIR="build"
	else
		SITE_DIR="build/${lang}"
	fi

	# Generate mkdocs.yaml
	${GENERATOR_DIR}/buildyaml.sh ${MKDOCS_DIR} ${SITE_DIR} ${lang}>${MKDOCS_CONFIG_FILE}

	echo "Fixing links"

	# Fix links (recursively, all types, executing replacements)
	${GENERATOR_DIR}/checklinks.sh -rax

	echo "Calling mkdocs"

	# Build html docs
	mkdocs build --config-file="${MKDOCS_CONFIG_FILE}"

	# Fix edit buttons for the markdowns that are not on the main Netdata repo
	find "${GENERATOR_DIR}/${SITE_DIR}/${GO_D_DIR}" -name "*.html" -print0 | xargs -0 sed -i -e 's/https:\/\/github.com\/netdata\/netdata\/blob\/master\/collectors\/go.d.plugin/https:\/\/github.com\/netdata\/go.d.plugin\/blob\/master/g'
	if [ "${lang}" != "en" ] ; then
		find "${GENERATOR_DIR}/${SITE_DIR}" -name "*.html" -print0 | xargs -0 sed -i -e 's/https:\/\/github.com\/netdata\/netdata\/blob\/master\/\S*md/https:\/\/github.com\/netdata\/localization\//g'
	fi

	# Replace index.html with DOCUMENTATION/index.html. Since we're moving it up one directory, we need to remove ../ from the links
	echo "Replacing index.html with DOCUMENTATION/index.html"
	sed 's/\.\.\///g' ${GENERATOR_DIR}/${SITE_DIR}/DOCUMENTATION/index.html > ${GENERATOR_DIR}/${SITE_DIR}/index.html

}

for d in "en" "zh" "pt" ; do
	echo "Preparing source for $d"
	cp -r ${SRC_DIR} ${DOCS_DIR}
	if [ "${d}" != "en" ] ; then
		cp -a ${LOC_DIR}/${d}/* ${DOCS_DIR}/
	fi
	prep_html $d
	rm -rf ${DOCS_DIR}
done

# Remove cloned projects and temp directories
rm -rf ${GO_D_DIR} ${LOC_DIR} ${DOCS_DIR} ${SRC_DIR}

echo "Finished"
