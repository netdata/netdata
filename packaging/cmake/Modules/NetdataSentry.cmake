# SPDX-License-Identifier: GPL-3.0-or-later
# Functions and macros for handling of Sentry
#
# Handle bundling of Sentry.
#
# This pulls it in as a sub-project using FetchContent functionality.
#
# This needs to be a function and not a macro for variable scoping
# reasons. All the things we care about from the sub-project are exposed
# as targets, which are globally scoped and not function scoped.
function(netdata_bundle_sentry)
        include(FetchContent)

        # ignore debhelper
        set(FETCHCONTENT_FULLY_DISCONNECTED Off)

        set(SENTRY_VERSION 0.13.5)
        set(SENTRY_BACKEND "breakpad")
        set(SENTRY_BUILD_SHARED_LIBS OFF)

        FetchContent_Declare(
                sentry
                GIT_REPOSITORY https://github.com/getsentry/sentry-native.git
                GIT_TAG 6ebd29bd9742fd2f93b6770b5023e31a8efbc10e # v0.13.5
                CMAKE_ARGS ${NETDATA_CMAKE_PROPAGATE_TOOLCHAIN_ARGS}
        )
        FetchContent_MakeAvailable(sentry)
endfunction()
