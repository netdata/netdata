#!/bin/sh

set -e

REPO="${1}"
EVENT_NAME="${2}"
EVENT_TYPE="${3}"
EVENT_VERSION="${4}"
RELEASE_TEST="${5}"

# Git refs cannot contain '~', so accept the package-style RC separator and store the tag form.
# The build system maps '-rcN' back to '~rcN' for native package versions.
case "${EVENT_VERSION}" in
    *\~rc*) EVENT_VERSION="${EVENT_VERSION%%\~rc*}-rc${EVENT_VERSION#*\~rc}" ;;
esac

##############################################################
# Version validation functions

check_version_format() {
    if ! echo "${EVENT_VERSION}" | grep -qE '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'; then
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
    if ! echo "${EVENT_VERSION}" | grep -qE '^v[[:digit:]]+\.0\.';  then
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
    if git show-ref --verify --quiet "refs/tags/${EVENT_VERSION}"; then
        echo "::error::A tag for version ${EVENT_VERSION} already exists."
        return 1
    fi
}

check_newer_version() {
    current="$(cut -f 1 -d '-' packaging/version)"
    target="${EVENT_VERSION%%-*}"

    if [ "${target}" = "${current}" ] || [ "$(printf '%s\n' "${current}" "${target}" | sort -V | tail -n 1)" != "${target}" ]; then
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
    # Release candidates are tagged on master; nightlies carry only the numeric core.
    # Zero-pad the count so nightly versions also sort correctly as text.
    NEW_VERSION="${LAST_TAG%%-rc*}-$(printf '%05d' "$((COMMITS_SINCE_RELEASE + 1))")-nightly"
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
        echo "channel=nightly" >> "${GITHUB_OUTPUT}"
        echo "branch=master" >> "${GITHUB_OUTPUT}"
        echo "version=nightly" >> "${GITHUB_OUTPUT}"
    fi
elif [ "${EVENT_VERSION}" != nightly ]; then
    case "${EVENT_TYPE}" in
        release-candidate)
            if ! echo "${EVENT_VERSION}" | grep -qE '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.0-rc[1-9][0-9]*$'; then
                echo '::error::Release candidates must use vMAJOR.MINOR.0-rcX, with positive X and no build metadata.'
                exit 1
            fi
            ;;
        major|minor|patch)
            check_version_format || exit 1
            if [ "${EVENT_TYPE}" != patch ]; then patch_is_zero || exit 1; fi
            if [ "${EVENT_TYPE}" = major ]; then minor_is_zero || exit 1; fi
            ;;
        *)
            echo '::error::Unrecognized release type.'
            exit 1
            ;;
    esac
    check_for_existing_tag || exit 1
    branch_name="$(echo "${EVENT_VERSION}" | cut -f 1-2 -d '.')"
    new_branch=''
    build_type=release
    if [ "${EVENT_TYPE}" = release-candidate ]; then
        build_type=release-candidate
        base_version="${EVENT_VERSION%-rc*}"
        # Match the full core literally, including stable tags with build metadata.
        if git tag | cut -d "+" -f 1 | grep -F -x -q "${base_version}"; then
            echo "::error::A regular release of ${base_version} already exists."
            exit 1
        fi
        latest_rc="$(git tag --list "${base_version}-rc*" | grep -E '^v[0-9]+\.[0-9]+\.0-rc[1-9][0-9]*$' | sort -V | tail -n 1)"
        if [ -z "${latest_rc}" ]; then
            if [ "${EVENT_VERSION}" != "${base_version}-rc1" ]; then
                echo '::error::Release candidate numbering must start at rc1.'
                exit 1
            fi
            if git show-ref --verify --quiet "refs/heads/${branch_name}" || git show-ref --verify --quiet "refs/remotes/origin/${branch_name}"; then
                echo "::error::A branch named ${branch_name} already exists in the repository."
                exit 1
            fi
            git checkout master
            # A first RC starts a new major or minor series from master.
            current_major="$(cut -f 1 -d '-' packaging/version | cut -f 1 -d '.')"
            target_major="$(echo "${EVENT_VERSION}" | cut -f 1 -d '.')"
            check_newer_version || exit 1
            if [ "${current_major}" != "${target_major}" ]; then minor_is_zero || exit 1; fi
            new_branch="${branch_name}"
            branch=master
        else
            # Version sort compares arbitrary-length decimal RC numbers numerically.
            if [ "$(printf '%s\n' "${latest_rc}" "${EVENT_VERSION}" | sort -V | tail -n 1)" != "${EVENT_VERSION}" ]; then
                echo "::error::Release candidate must be newer than ${latest_rc}."
                exit 1
            fi
            branch="${branch_name}"
        fi
    else
        branch="${branch_name}"
    fi
    if [ -z "${new_branch}" ]; then
        if ! git checkout "${branch_name}"; then
            echo "::error::Could not find a branch for the ${branch_name}.x release series."
            exit 1
        fi
        major_matches || exit 1
        minor_matches || exit 1
        if [ "${EVENT_TYPE}" = patch ]; then
            check_newer_version || exit 1
        elif [ "${EVENT_TYPE}" != release-candidate ]; then
            case "$(cat packaging/version)" in
                "${EVENT_VERSION}"-*) ;; # Same-core prerelease to final release.
                *) check_newer_version || exit 1 ;;
            esac
        fi
    fi
    echo "${EVENT_VERSION}" > packaging/version || exit 1
    # shellcheck disable=SC2129
    echo "run=true" >> "${GITHUB_OUTPUT}"
    echo "message=${EVENT_TYPE} release ${EVENT_VERSION}." >> "${GITHUB_OUTPUT}"
    echo "ref=${EVENT_VERSION}" >> "${GITHUB_OUTPUT}"
    echo "type=${build_type}" >> "${GITHUB_OUTPUT}"
    echo "channel=stable" >> "${GITHUB_OUTPUT}"
    echo "branch=${branch}" >> "${GITHUB_OUTPUT}"
    if [ -n "${new_branch}" ]; then echo "new-branch=${new_branch}" >> "${GITHUB_OUTPUT}"; fi
    echo "version=${EVENT_VERSION#v}" >> "${GITHUB_OUTPUT}"
else
    echo '::error::Unrecognized release type or invalid version.'
    exit 1
fi
