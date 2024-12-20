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

        set(SENTRY_VERSION 0.6.6)
        set(SENTRY_BACKEND "breakpad")
        set(SENTRY_BUILD_SHARED_LIBS OFF)

        FetchContent_Declare(
                sentry
                GIT_REPOSITORY https://github.com/getsentry/sentry-native.git
                GIT_TAG c97bcc63fa89ae557cef9c9b6e3acb11a72ff97d # v0.6.6
                CMAKE_ARGS ${NETDATA_CMAKE_PROPAGATE_TOOLCHAIN_ARGS}
        )
        FetchContent_MakeAvailable(sentry)
endfunction()
