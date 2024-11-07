#!/bin/sh

set -e

REPO="${1}"
EVENT_NAME="${2}"
EVENT_TYPE="${3}"
EVENT_VERSION="${4}"
RELEASE_TEST="${5}"

##############################################################
# Version validation functions

check_version_format() {
    if ! echo "${EVENT_VERSION}" | grep -qE '^v[[:digit:]]+\.[[:digit:]]+\.[[:digit:]]+$'; then
        echo "::error::The supplied version (${EVENT_VERSION}) is not a valid version string."
        return 1
    fi
}

patch_is_zero() {
    if ! echo "${EVENT_VERSION}" | grep -qE '^v[[:digit:]]+\.[[:digit:]]+\.0$'; then
        echo "::error::The patch number for a ${EVENT_TYPE} build must be 0."
        return 1
    fi
}

minor_is_zero() {
    if ! echo "${EVENT_VERSION}" | grep -qE '^v[[:digit:]]+\.0';  then
        echo "::error::The minor version number for a ${EVENT_TYPE} build must be 0."
        return 1
    fi
}

major_matches() {
    current_major="$(cut -f 1 -d '-' packaging/version | cut -f 1 -d '.' | cut -f 2 -d 'v')"
    target_major="$(echo "${EVENT_VERSION}" | cut -f 1 -d '.' | cut -f 2 -d 'v')"

    if [ "${target_major}" != "${current_major}" ]; then
        echo "::error::Major version mismatch, expected ${current_major} but got ${target_major}."
        return 1
    fi
}

minor_matches() {
    current_minor="$(cut -f 1 -d '-' packaging/version | cut -f 2 -d '.')"
    target_minor="$(echo "${EVENT_VERSION}" | cut -f 2 -d '.')"

    if [ "${target_minor}" != "${current_minor}" ]; then
        echo "::error::Minor version mismatch, expected ${current_minor} but got ${target_minor}."
        return 1
    fi
}

check_for_existing_tag() {
    if git tag | grep -qE "^${EVENT_VERSION}$"; then
        echo "::error::A tag for version ${EVENT_VERSION} already exists."
        return 1
    fi
}

check_newer_major_version() {
    current="$(cut -f 1 -d '-' packaging/version | cut -f 1 -d '.' | cut -f 2 -d 'v')"
    target="$(echo "${EVENT_VERSION}" | cut -f 1 -d '.' | cut -f 2 -d 'v')"

    if [ "${target}" -le "${current}" ]; then
        echo "::error::Version ${EVENT_VERSION} is not newer than the current version."
        return 1
    fi
}

check_newer_minor_version() {
    current="$(cut -f 1 -d '-' packaging/version | cut -f 2 -d '.')"
    target="$(echo "${EVENT_VERSION}" | cut -f 2 -d '.')"

    if [ "${target}" -le "${current}" ]; then
        echo "::error::Version ${EVENT_VERSION} is not newer than the current version."
        return 1
    fi
}

check_newer_patch_version() {
    current="$(cut -f 1 -d '-' packaging/version | cut -f 3 -d '.')"
    target="$(echo "${EVENT_VERSION}" | cut -f 3 -d '.')"

    if [ "${target}" -le "${current}" ]; then
        echo "::error::Version ${EVENT_VERSION} is not newer than the current version."
        return 1
    fi
}

##############################################################
# Core logic

git config user.name "netdatabot"
git config user.email "bot@netdata.cloud"

if [ "${REPO}" != "netdata/netdata" ] && [ -z "${RELEASE_TEST}" ]; then
    echo "::notice::Not running in the netdata/netdata repository, not queueing a release build."
    echo "run=false" >> "${GITHUB_OUTPUT}"
elif [ "${EVENT_NAME}" = 'schedule' ] || [ "${EVENT_TYPE}" = 'nightly' ]; then
    echo "::notice::Preparing a nightly release build."
    LAST_TAG=$(git describe --abbrev=0 --tags)
    COMMITS_SINCE_RELEASE=$(git rev-list "${LAST_TAG}"..HEAD --count)
    NEW_VERSION="${LAST_TAG}-$((COMMITS_SINCE_RELEASE + 1))-nightly"
    LAST_VERSION_COMMIT="$(git rev-list -1 HEAD packaging/version)"
    HEAD_COMMIT="$(git rev-parse HEAD)"
    if [ "${EVENT_NAME}" = 'schedule' ] && [ "${LAST_VERSION_COMMIT}" = "${HEAD_COMMIT}" ] && grep -qE '.*-nightly$' packaging/version; then
        echo "::notice::No commits since last nightly build, not publishing a new nightly build."
        echo "run=false" >> "${GITHUB_OUTPUT}"
    else
        echo "${NEW_VERSION}" > packaging/version || exit 1
        # shellcheck disable=SC2129
        echo "run=true" >> "${GITHUB_OUTPUT}"
        echo "message=Update changelog and version for nightly build: ${NEW_VERSION}." >> "${GITHUB_OUTPUT}"
        echo "ref=master" >> "${GITHUB_OUTPUT}"
        echo "type=nightly" >> "${GITHUB_OUTPUT}"
        echo "branch=master" >> "${GITHUB_OUTPUT}"
        echo "version=nightly" >> "${GITHUB_OUTPUT}"
    fi
elif [ "${EVENT_TYPE}" = 'patch' ] && [ "${EVENT_VERSION}" != "nightly" ]; then
    echo "::notice::Preparing a patch release build."
    check_version_format || exit 1
    check_for_existing_tag || exit 1
    branch_name="$(echo "${EVENT_VERSION}" | cut -f 1-2 -d '.')"
    if ! git checkout "${branch_name}"; then
        echo "::error::Could not find a branch for the ${branch_name}.x release series."
        exit 1
    fi
    minor_matches || exit 1
    major_matches || exit 1
    check_newer_patch_version || exit 1
    echo "${EVENT_VERSION}" > packaging/version || exit 1
    # shellcheck disable=SC2129
    echo "run=true" >> "${GITHUB_OUTPUT}"
    echo "message=Patch release ${EVENT_VERSION}." >> "${GITHUB_OUTPUT}"
    echo "ref=${EVENT_VERSION}" >> "${GITHUB_OUTPUT}"
    echo "type=release" >> "${GITHUB_OUTPUT}"
    echo "branch=${branch_name}" >> "${GITHUB_OUTPUT}"
    echo "version=$(tr -d 'v' < packaging/version)" >> "${GITHUB_OUTPUT}"
elif [ "${EVENT_TYPE}" = 'minor' ] && [ "${EVENT_VERSION}" != "nightly" ]; then
    echo "::notice::Preparing a minor release build."
    check_version_format || exit 1
    patch_is_zero || exit 1
    major_matches || exit 1
    check_newer_minor_version || exit 1
    check_for_existing_tag || exit 1
    branch_name="$(echo "${EVENT_VERSION}" | cut -f 1-2 -d '.')"
    if [ -n "$(git branch --list "${branch_name}")" ]; then
        echo "::error::A branch named ${branch_name} already exists in the repository."
        exit 1
    fi
    echo "${EVENT_VERSION}" > packaging/version || exit 1
    # shellcheck disable=SC2129
    echo "run=true" >> "${GITHUB_OUTPUT}"
    echo "message=Minor release ${EVENT_VERSION}." >> "${GITHUB_OUTPUT}"
    echo "ref=${EVENT_VERSION}" >> "${GITHUB_OUTPUT}"
    echo "type=release" >> "${GITHUB_OUTPUT}"
    echo "branch=master" >> "${GITHUB_OUTPUT}"
    echo "new-branch=${branch_name}" >> "${GITHUB_OUTPUT}"
    echo "version=$(tr -d 'v' < packaging/version)" >> "${GITHUB_OUTPUT}"
elif [ "${EVENT_TYPE}" = 'major' ] && [ "${EVENT_VERSION}" != "nightly" ]; then
    echo "::notice::Preparing a major release build."
    check_version_format || exit 1
    minor_is_zero || exit 1
    patch_is_zero || exit 1
    check_newer_major_version || exit 1
    check_for_existing_tag || exit 1
    branch_name="$(echo "${EVENT_VERSION}" | cut -f 1-2 -d '.')"
    if [ -n "$(git branch --list "${branch_name}")" ]; then
        echo "::error::A branch named ${branch_name} already exists in the repository."
        exit 1
    fi
    echo "${EVENT_VERSION}" > packaging/version || exit 1
    # shellcheck disable=SC2129
    echo "run=true" >> "${GITHUB_OUTPUT}"
    echo "message=Major release ${EVENT_VERSION}" >> "${GITHUB_OUTPUT}"
    echo "ref=${EVENT_VERSION}" >> "${GITHUB_OUTPUT}"
    echo "type=release" >> "${GITHUB_OUTPUT}"
    echo "branch=master" >> "${GITHUB_OUTPUT}"
    echo "new-branch=${branch_name}" >> "${GITHUB_OUTPUT}"
    echo "version=$(tr -d 'v' < packaging/version)" >> "${GITHUB_OUTPUT}"
else
    echo '::error::Unrecognized release type or invalid version.'
    exit 1
fi
