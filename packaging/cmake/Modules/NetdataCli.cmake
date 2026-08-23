# SPDX-License-Identifier: GPL-3.0-or-later
# netdatacli: the command-line client for the running agent.
#
# include()d from the root file, so paths resolve against the repository and
# build roots; nothing here may use CMAKE_CURRENT_LIST_DIR.

include_guard()

set(NETDATACLI_FILES
        src/daemon/commands.h
        src/daemon/pipename.c
        src/daemon/pipename.h
        src/cli/cli.c
)

# windows: the netdatacli resource script
set(NETDATACLI_RES_FILES "packaging/windows/resources/netdatacli.rc")

if(OS_WINDOWS)
    configure_file(packaging/windows/resources/netdatacli.manifest.in ${CMAKE_SOURCE_DIR}/packaging/windows/resources/netdatacli.manifest @ONLY)
endif()

#
# build netdatacli
#

add_executable(netdatacli ${NETDATACLI_FILES} "$<$<BOOL:${OS_WINDOWS}>:${NETDATACLI_RES_FILES}>")
target_link_libraries(netdatacli libnetdata)

install(TARGETS netdatacli
        COMPONENT netdata
        DESTINATION "${BINDIR}")
