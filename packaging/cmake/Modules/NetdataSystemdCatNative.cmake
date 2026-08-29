# SPDX-License-Identifier: GPL-3.0-or-later
# systemd-cat-native: sends log entries to a local or remote systemd journal.
#
# include()d from the root file, so paths resolve against the repository and
# build roots; nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# libcurl is a hard requirement of the build, checked in NetdataDependencies,
# so this links PkgConfig::CURL unconditionally.

include_guard()

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
