# SPDX-License-Identifier: GPL-3.0-or-later
# systemd-cat-native: sends log entries to a local or remote systemd journal.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# libcurl is a hard requirement of the build, checked in NetdataDependencies,
# so this links PkgConfig::CURL unconditionally.

set(SYSTEMD_CAT_NATIVE_FILES src/libnetdata/log/systemd-cat-native.c
                             src/libnetdata/log/systemd-cat-native.h)

#
# build systemd-cat-native
#

add_executable(systemd-cat-native ${SYSTEMD_CAT_NATIVE_FILES})
target_link_libraries(systemd-cat-native
        libnetdata
        PkgConfig::CURL
)

install(TARGETS systemd-cat-native
        COMPONENT netdata
        DESTINATION "${BINDIR}")
