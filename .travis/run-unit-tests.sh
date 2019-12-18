#!/usr/bin/env bash
#
# Unit-testing script
#
# This script does the following:
#   1. Check whether any files were modified that would necessitate unit testing (using the `TRAVIS_COMMIT_RANGE` environment variable).
#   2. If there are no changed files that require unit testing, exit successfully.
#   3. Otherwise, run all the unit tests.
#
# We do things this way because our unit testing takes a rather long
# time (average 18-19 minutes as of the original creation of this script),
# so skipping it when we don't actually need it can significantly speed
# up the CI process.
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author: Austin S. Hemmelgarn <austin@netdata.cloud>
#
# shellcheck disable=SC2230

install_netdata() {
    echo "Installing Netdata"
    fakeroot ./netdata-installer.sh --install $HOME --dont-wait --dont-start-it --enable-plugin-nfacct --enable-plugin-freeipmi --disable-lto
}

c_unit_tests() {
    echo "Running C code unit tests"
    $HOME/netdata/usr/sbin/netdata -W unittest
}

run_c_unit_tests=

if [ -z ${TRAVIS_COMMIT_RANGE} ] ; then
    # Travis gave us no commit range, so just run all the unit tests.
    # Per the docs, this is the case when a new branch is pushed for the first time.
    echo "No commit range supplied, assuming the worst case and running all unit tests."
    run_c_unit_tests=1
else
    changed_paths=

    if [ "${TRAVIS_PULL_REQUEST}" = "false" ] ; then
        # This is not a PR build.
        COMMIT1="$(echo ${TRAVIS_COMMIT_RANGE} | cut -f 1 -d '.')"
        COMMIT2="$(echo ${TRAVIS_COMMIT_RANGE} | cut -f 4 -d '.')"

        if [ "$(git cat-file -t ${COMMIT1} 2>/dev/null)" = commit -a "$(git cat-file -t ${COMMIT2} 2>/dev/null)" = commit ] ; then
            # Examine the exact set of commits passed by Travis.
            echo "Checking commits:"
            git log --format=oneline --abbrev-commit ${COMMIT1}..${COMMIT2}
            changed_paths="$(git diff --name-only ${COMMIT1}..${COMMIT2} --)"
        else
            # We couldn't find at least one of the changesets, so this build
            # was probably triggered by a history rewrite. Since we can't
            # figure out what chnaged, we need to just run all the tests anyway.
            echo "Cannot determine which commits we are testing, running all unit tests."
            run_c_unit_tests=1
        fi
    else
        # This is a PR build, look at all commits from the target branch
        # to HEAD.
        echo "Checking commits:"
        git log --format=oneline --abbrev-commit ${TRAVIS_BRANCH}..HEAD
        changed_paths="$(git diff --name-only ${TRAVIS_BRANCH}..HEAD --)"
    fi

    if [ -n "${changed_paths}" ] ; then
        # Check for changes that would require the C code to be re-tested
        if (echo ${changed_paths} | grep -qE "daemon/unit_test|database") ; then
            echo "Commits appear to change C code with unit tests, queueing C unit tests."
            run_c_unit_tests=1
        fi
    fi
fi

if [ -z ${run_c_unit_tests} ] ; then
    # No tests to run, log this and exit with success
    echo "Commit range ${TRAVIS_COMMIT_RANGE} appears to make no changes that require unit tests, skipping unit testing."
    exit 0
else
    install_netdata || exit 1

    if [ -n ${run_c_unit_tests} ] ; then
        c_unit_tests || exit 1
    fi
fi
