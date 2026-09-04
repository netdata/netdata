#!/usr/bin/env bash

set -eou pipefail

artifacts="$(realpath "${1}")"
signing_key="${2:-}"

TOP="$(pwd)"
DISTFILE_EXTENSIONS="gz"
MSI_ARCHES="x64"
STATIC_ARCHES="x86_64 aarch64 armv6l armv7l"
STATIC_EXTENSIONS="gz"
VERSION="$(cat packaging/version)"

copy_static_builds() {
    for ext in ${STATIC_EXTENSIONS}; do
        for arch in ${STATIC_ARCHES}; do
            for tag in "${@}"; do
                cp -v "${artifacts}/netdata-${arch}-latest.${ext}.run" "./netdata-${arch}-${tag}.${ext}.run"
            done
        done
    done
}

copy_source_tarball() {
    for ext in ${DISTFILE_EXTENSIONS} ; do
        distfile="$(shopt -s nullglob; set -- "${artifacts}"/netdata-*.tar."${ext}"; echo "${1:-}")"
        if [ -z "${distfile}" ]; then
            echo "::warning title=No ${ext} distfile matched::Failed to match any source tarball with a ${ext} extension when preparing artifacts."
            return 1
        fi

        for tag in "${@}"; do
            case "${tag}" in
                '') cp -v "${distfile}" . ;;
                *) cp -v "${distfile}" "./netdata-${tag}.tar.${ext}" ;;
            esac
        done
    done
}

copy_msi_packages() {
    for arch in ${MSI_ARCHES} ; do
        for tag in "${@}" ; do
            case "${tag}" in
                '') cp -v "${artifacts}/netdata-${arch}.msi" . ;;
                *) cp -v "${artifacts}/netdata-${arch}.msi" "./netdata-${tag}-${arch}.msi" ;;
            esac
        done
    done
}

create_legacy_compat_files() {
    ln -s ./netdata-x86_64-latest.gz.run netdata-latest.gz.run
    ln -s "./netdata-x86_64-${VERSION}.gz.run" "netdata-${VERSION}.gz.run"
    echo "${VERSION}" > latest-version.txt
    sha256sum -b ./* > sha256sums.txt
}

create_manifest() {
    for f in * ; do
        [ -f "${f}" ] || continue
        sha256="$(sha256sum -b "$f" | awk '{print $1}')"
        size="$(wc -c < "$f" | awk '{print $1}')"
        printf '%s %s %s\n' "${f}" "${size}" "${sha256}" >> Manifest
    done
    if [ -n "${signing_key}" ]; then
        gpg -u "${signing_key}" --detach-sign Manifest
    fi
}

echo "Using ${artifacts} as source directory for artifacts"
echo "::group::Files currently in ${artifacts}"
ls -l "${artifacts}"
echo "::endgroup::"

echo "::group::Preparing GitHub release artifacts"
mkdir -p artifacts/github
cd artifacts/github
copy_source_tarball "" "${VERSION}" latest
copy_static_builds "${VERSION}" latest
copy_msi_packages "" "${VERSION}" latest
create_legacy_compat_files
echo "${VERSION}" > Version
create_manifest
cat Manifest
cd "${TOP}"
echo "::endgroup::"

echo "::group::Preparing R2 versioned release artifacts"
mkdir -p "artifacts/r2/${VERSION}"
cd "artifacts/r2/${VERSION}"
copy_source_tarball "${VERSION}"
copy_static_builds "${VERSION}" latest
copy_msi_packages "${VERSION}"
echo "${VERSION}" > Version
create_manifest
cat Manifest
cd "${TOP}"
echo "::endgroup::"

echo "::group::Preparing R2 latest release artifacts"
mkdir -p artifacts/r2/latest
cd artifacts/r2/latest
copy_source_tarball latest
copy_static_builds latest
copy_msi_packages latest
echo "${VERSION}" > Version
create_manifest
cat Manifest
cd "${TOP}"
echo "::endgroup::"
