# SPDX-License-Identifier: GPL-3.0-or-later
# Function to provide information regarding the Netdata version.
#
# The high-level logic is (a) use git-describe, (b) fallback to info from
# packaging/version. This version field are used for cmake's project,
# cpack's packaging, and the agent's functionality.
function(netdata_version)
        find_package(Git)

        if(GIT_EXECUTABLE)
                execute_process(COMMAND ${GIT_EXECUTABLE} describe
                                WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}
                                RESULT_VARIABLE GIT_DESCRIBE_RESULT
                                OUTPUT_VARIABLE GIT_DESCRIBE_OUTPUT)
                if(GIT_DESCRIBE_RESULT)
                        file(STRINGS "${CMAKE_SOURCE_DIR}/packaging/version" GIT_DESCRIBE_OUTPUT)
                        message(WARNING "using version from packaging/version: '${GIT_DESCRIBE_OUTPUT}'")
                endif()
        else()
                file(STRINGS packaging/version GIT_DESCRIBE_OUTPUT)
                message(WARNING "using version from packaging/version: '${GIT_DESCRIBE_OUTPUT}'")
        endif()

        string(STRIP ${GIT_DESCRIBE_OUTPUT} GIT_DESCRIBE_OUTPUT)
        set(NETDATA_VERSION_STRING "${GIT_DESCRIBE_OUTPUT}" PARENT_SCOPE)

        string(REGEX MATCH "v?([0-9]+)\\.([0-9]+)\\.([0-9]+)-?([0-9]+)?-?([0-9a-zA-Z]+)?" MATCHES "${GIT_DESCRIBE_OUTPUT}")
        if(CMAKE_MATCH_COUNT EQUAL 3)
                set(NETDATA_VERSION_MAJOR ${CMAKE_MATCH_1} PARENT_SCOPE)
                set(NETDATA_VERSION_MINOR ${CMAKE_MATCH_2} PARENT_SCOPE)
                set(NETDATA_VERSION_PATCH ${CMAKE_MATCH_3} PARENT_SCOPE)
                set(NETDATA_VERSION_TWEAK 0 PARENT_SCOPE)
                set(NETDATA_VERSION_DESCR "N/A" PARENT_SCOPE)
        elseif(CMAKE_MATCH_COUNT EQUAL 4)
                set(NETDATA_VERSION_MAJOR ${CMAKE_MATCH_1} PARENT_SCOPE)
                set(NETDATA_VERSION_MINOR ${CMAKE_MATCH_2} PARENT_SCOPE)
                set(NETDATA_VERSION_PATCH ${CMAKE_MATCH_3} PARENT_SCOPE)
                set(NETDATA_VERSION_TWEAK ${CMAKE_MATCH_4} PARENT_SCOPE)
                set(NETDATA_VERSION_DESCR "N/A" PARENT_SCOPE)
        elseif(CMAKE_MATCH_COUNT EQUAL 5)
                set(NETDATA_VERSION_MAJOR ${CMAKE_MATCH_1} PARENT_SCOPE)
                set(NETDATA_VERSION_MINOR ${CMAKE_MATCH_2} PARENT_SCOPE)
                set(NETDATA_VERSION_PATCH ${CMAKE_MATCH_3} PARENT_SCOPE)
                set(NETDATA_VERSION_TWEAK ${CMAKE_MATCH_4} PARENT_SCOPE)
                set(NETDATA_VERSION_DESCR ${CMAKE_MATCH_5} PARENT_SCOPE)
        else()
                message(FATAL_ERROR "Wrong version regex match count ${CMAKE_MATCH_COUNT} (should be in 3, 4 or 5)")
        endif()
endfunction()
