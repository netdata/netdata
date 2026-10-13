#!/usr/bin/env bash

set -euo pipefail

artifacts="$(realpath "${1}")"
signing_key="${2:-}"
event_type="${3:-}"
build_type="${4:-nightly}"

case "${build_type}" in
    release) build_type='stable' ;;
esac

TOP="$(pwd)"
DISTFILE_EXTENSIONS="gz zst"
MSI_ARCHES="x64"
STATIC_ARCHES="x86_64 aarch64 armv6l armv7l"
STATIC_EXTENSIONS="gz"
VERSION="$(cat packaging/version)"
VERSION_URL_PREFIX="https://artifacts.netdata.cloud/${build_type}"

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
        distfile="$(shopt -s nullglob; for candidate in "${artifacts}"/netdata-*.tar."${ext}"; do if [[ -f "${candidate}" && -r "${candidate}" ]]; then printf '%s' "${candidate}"; break; fi; done)"
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
    rm -f Manifest Manifest.sig
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

# Semantic equivalent to X >= Y for versions.
# Release candidates always sort lower than non-release candidates of the same version.
version_compare() {
    local v1 v2
    local IFS=.-

    read -ra v1 <<< "${1#v}"
    read -ra v2 <<< "${2#v}"

    if (( 10#${v1[0]} > 10#${v2[0]} )); then return 0; fi
    if (( 10#${v1[0]} < 10#${v2[0]} )); then return 1; fi

    if (( 10#${v1[1]} > 10#${v2[1]} )); then return 0; fi
    if (( 10#${v1[1]} < 10#${v2[1]} )); then return 1; fi

    if (( 10#${v1[2]} > 10#${v2[2]} )); then return 0; fi
    if (( 10#${v1[2]} < 10#${v2[2]} )); then return 1; fi

    if [[ "${v1[3]:-0}" == rc* ]]; then
        v1rc=1
    else
        v1rc=0
    fi

    if [[ "${v2[3]:-0}" == rc* ]]; then
        v2rc=1
    else
        v2rc=0
    fi

    if [ "${v1rc}" -eq 1 ] && [ "${v2rc}" -eq 1 ]; then
        if (( 10#${v1[3]#rc} > 10#${v2[3]#rc} )); then return 0; fi
        if (( 10#${v1[3]#rc} < 10#${v2[3]#rc} )); then return 1; fi
    fi

    if [ "${v1rc}" -ne 1 ] && [ "${v2rc}" -eq 1 ]; then return 0; fi
    if [ "${v1rc}" -eq 1 ] && [ "${v2rc}" -ne 1 ]; then return 1; fi

    return 0
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
copy_static_builds "${VERSION}"
copy_msi_packages "${VERSION}"
echo "${VERSION}" > Version
create_manifest
cat Manifest
cd "${TOP}"
echo "::endgroup::"

prepare_latest=0
if [ "${event_type}" != 'pull_request' ] && [ "${build_type}" != 'nightly' ]; then
    dl_log="./dl.log"

    rm -f "${dl_log}"
    set +e
    wget -S -o "${dl_log}" "${VERSION_URL_PREFIX}/latest/Version"
    ret="$?"
    set -e

    case "${ret}" in
        0)
            if version_compare "$(cat Version)" "${VERSION}"; then
                prepare_latest=0
            else
                prepare_latest=1
            fi
            ;;
        8)
            case "$(grep "HTTP/" "${dl_log}" | tail -n 1 | awk '{ print $2 }')" in
                404) prepare_latest=1 ;;
                *)
                    echo "::error::Failed to determine latest published ${build_type} version."
                    cat "${dl_log}"
                    exit 1
                    ;;
            esac
            ;;
        *)
            echo "::error::Failed to determine latest published ${build_type} version."
                    cat "${dl_log}"
            exit 1
            ;;
    esac

    rm -f Version

    if [ "${build_type}" != "release-candidate" ]; then
        major="$(echo "${VERSION}" | tr -d 'v' | cut -f 1 -d '.')"
        minor="$(echo "${VERSION}" | tr -d 'v' | cut -f 2 -d '.')"
        for t in "${major}" "${major}.${minor}" ; do
            rm -f "${dl_log}"
            set +e
            wget -S -o "${dl_log}" "${VERSION_URL_PREFIX}/${t}/Version"
            ret="$?"
            set -e

            case "${ret}" in
                0)
                    if version_compare "$(cat Version)" "${VERSION}"; then
                        prepare_target=0
                    else
                        prepare_target=1
                    fi
                    ;;
                8)
                    case "$(grep "HTTP/" "${dl_log}" | tail -n 1 | awk '{ print $2 }')" in
                        404) prepare_target=1 ;;
                        *)
                            echo "::error::Failed to determine latest published ${build_type} v${t} version."
                            cat "${dl_log}"
                            exit 1
                            ;;
                    esac
                    ;;
                *)
                    echo "::error::Failed to determine latest published ${build_type} v${t} version."
                    cat "${dl_log}"
                    exit 1
                    ;;
            esac

            if [ "${prepare_target}" -eq 1 ]; then
                echo "::group::Preparing R2 secondary versioned release artifacts (${t})"
                cp -va "artifacts/r2/${VERSION}" "artifacts/r2/${t}"
                echo "::endgroup::"
            fi

            rm -f Version
        done
    fi
else
    prepare_latest=1
fi

if [ "${prepare_latest}" -eq 1 ]; then
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
fi
