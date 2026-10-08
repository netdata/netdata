# SPDX-License-Identifier: GPL-3.0-or-later
# local-listeners: the listening-socket enumerator, and its test binaries.
#
# include()d from the root file, so paths resolve against the repository and
# build roots; nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# The test targets live here rather than in NetdataTests.cmake with the
# other test binaries. They are EXCLUDE_FROM_ALL and never installed, but
# they exercise the same local-sockets code this module builds, so they are
# gated on the plugin option as well as the platform facts (the mnl test
# hard-links MNL and requires MNL_FOUND; the namespaces test only uses MNL
# when present).
#
# The source list lives here rather than in the shared
# NetdataSourceLists.cmake, so a plugin's whole definition is in one place. It
# sits above the guards, so it is defined unconditionally.

include_guard()

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

if(ENABLE_PLUGIN_LOCAL_LISTENERS AND OS_LINUX AND MNL_FOUND)
        add_executable(local-sockets-mnl-test EXCLUDE_FROM_ALL
                src/libnetdata/local-sockets/tests/test_local_sockets_mnl.c)
        target_compile_options(local-sockets-mnl-test PRIVATE ${MNL_CFLAGS_OTHER})
        target_include_directories(local-sockets-mnl-test PRIVATE ${MNL_INCLUDE_DIRS})
        target_link_libraries(local-sockets-mnl-test libnetdata ${MNL_LIBRARIES})
endif()

if(ENABLE_PLUGIN_LOCAL_LISTENERS AND OS_LINUX)
        add_executable(local-sockets-namespaces-test EXCLUDE_FROM_ALL
                src/libnetdata/local-sockets/tests/test_local_sockets_namespaces.c)
        target_compile_options(local-sockets-namespaces-test PRIVATE
                               "$<$<BOOL:${MNL_FOUND}>:${MNL_CFLAGS_OTHER}>")
        target_include_directories(local-sockets-namespaces-test PRIVATE
                                   "$<$<BOOL:${MNL_FOUND}>:${MNL_INCLUDE_DIRS}>")
        target_link_libraries(local-sockets-namespaces-test libnetdata
                              "$<$<BOOL:${MNL_FOUND}>:${MNL_LIBRARIES}>")
endif()
