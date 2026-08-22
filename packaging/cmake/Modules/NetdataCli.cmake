# SPDX-License-Identifier: GPL-3.0-or-later
# netdatacli: the command-line client for the running agent.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.

set(NETDATACLI_FILES
        src/daemon/commands.h
        src/daemon/pipename.c
        src/daemon/pipename.h
        src/cli/cli.c
)

# windows: the netdatacli resource script
set(NETDATACLI_RES_FILES "packaging/windows/resources/netdatacli.rc")

#
# build netdatacli
#

add_executable(netdatacli ${NETDATACLI_FILES} "$<$<BOOL:${OS_WINDOWS}>:${NETDATACLI_RES_FILES}>")
target_link_libraries(netdatacli libnetdata)

install(TARGETS netdatacli
        COMPONENT netdata
        DESTINATION "${BINDIR}")
