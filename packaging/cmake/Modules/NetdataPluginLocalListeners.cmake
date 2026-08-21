# SPDX-License-Identifier: GPL-3.0-or-later
# local-listeners: the listening-socket enumerator, and its mnl test binary.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# The test target travels with the plugin per D29. It is not installed and is
# EXCLUDE_FROM_ALL, but it exercises the same local-sockets code, so it
# belongs beside it rather than in NetdataTests.cmake - which would have meant
# re-creating its OS_LINUX AND MNL_FOUND guard at a new location, turning a
# provable move into a rewrite.
#
# Its source list moved here from NetdataSourceLists.cmake per D27, and stays
# above the guards so it is still defined unconditionally.

# local-listeners
set(LOCAL_LISTENERS_FILES
        src/collectors/utils/local_listeners.c
        src/libnetdata/local-sockets/local-sockets.h
)

if(ENABLE_PLUGIN_LOCAL_LISTENERS)
        add_executable(local-listeners ${LOCAL_LISTENERS_FILES})

        target_compile_options(local-listeners PRIVATE
                               "$<$<BOOL:${MNL_FOUND}>:${MNL_CFLAGS_OTHER}>")
        target_include_directories(local-listeners PRIVATE
                                   "$<$<BOOL:${MNL_FOUND}>:${MNL_INCLUDE_DIRS}>")
        target_link_libraries(local-listeners libnetdata
                              "$<$<BOOL:${MNL_FOUND}>:${MNL_LIBRARIES}>")

        install(TARGETS local-listeners
                COMPONENT netdata
                DESTINATION ${PLUGINS_DEST})
endif()

if(OS_LINUX AND MNL_FOUND)
        add_executable(local-sockets-mnl-test EXCLUDE_FROM_ALL
                src/libnetdata/local-sockets/tests/test_local_sockets_mnl.c)
        target_compile_options(local-sockets-mnl-test PRIVATE ${MNL_CFLAGS_OTHER})
        target_include_directories(local-sockets-mnl-test PRIVATE ${MNL_INCLUDE_DIRS})
        target_link_libraries(local-sockets-mnl-test libnetdata ${MNL_LIBRARIES})
endif()
