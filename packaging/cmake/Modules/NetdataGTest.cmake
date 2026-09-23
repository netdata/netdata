# SPDX-License-Identifier: GPL-3.0-or-later
# Functions and macros for handling of googletest

# googletest is a build-time-only dependency of the test executables. It is never linked into the agent, never
# installed, and ENABLE_GTEST defaults to Off, so a normal build neither fetches nor builds it. That is also why
# there is no system-package path here and no entry in REDISTRIBUTED.md: nothing of googletest reaches a shipped
# artifact, and pinning one revision keeps every machine and CI on the same source.

if(CMAKE_CXX_STANDARD LESS 17)
        message(FATAL_ERROR
                "ENABLE_GTEST requires C++17 or later, but CMAKE_CXX_STANDARD is ${CMAKE_CXX_STANDARD} "
                "(USE_CXX_11?). googletest 1.17.0 and newer require C++17; without this check the sub-project "
                "would quietly raise the standard of every target that links it.")
endif()

# Handle bundling of googletest.
#
# This pulls it in as a sub-project using FetchContent functionality.
#
# This needs to be a function and not a macro for variable scoping
# reasons. All the things we care about from the sub-project are exposed
# as targets, which are globally scoped and not function scoped.
function(netdata_bundle_gtest)
        include(FetchContent)
        include(NetdataFetchContentExtra)

        message(STATUS "Preparing vendored copy of googletest")

        # Never satisfy this from a system package: the tests are pinned to one googletest revision so that a
        # failure is reproducible everywhere, and a distro copy would silently change the framework under them.
        set(FETCHCONTENT_TRY_FIND_PACKAGE_MODE NEVER)

        set(FETCHCONTENT_FULLY_DISCONNECTED Off)

        # googletest defaults both of these to ON. We use neither gmock nor its install rules, and an install rule
        # would place googletest into the agent's install tree.
        set(BUILD_GMOCK OFF)
        set(INSTALL_GTEST OFF)
        set(BUILD_SHARED_LIBS OFF)

        set(repo https://github.com/google/googletest)
        set(tag 063de7e9578f82b369302001269680b4b1553359) # v1.18.0

        if(CMAKE_VERSION VERSION_GREATER_EQUAL 3.28)
                FetchContent_Declare(googletest
                        GIT_REPOSITORY ${repo}
                        GIT_TAG ${tag}
                        EXCLUDE_FROM_ALL
                )
        else()
                FetchContent_Declare(googletest
                        GIT_REPOSITORY ${repo}
                        GIT_TAG ${tag}
                )
        endif()

        FetchContent_MakeAvailable_NoInstall(googletest)

        message(STATUS "Finished preparing vendored copy of googletest")
endfunction()
