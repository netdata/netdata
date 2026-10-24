# SPDX-License-Identifier: GPL-3.0-or-later
# alarm-notify: the developer-only Go notifier. Built on request, never
# installed until the production migration.
#
# include()d from the root file, so paths resolve against the repository and
# build roots; nothing here may use CMAKE_CURRENT_LIST_DIR.

include_guard()

if(ENABLE_ALARM_NOTIFY_GO)
    include(NetdataGoTools)
    find_min_go_version("${CMAKE_SOURCE_DIR}/src/health/notifications/alarm-notify")
    find_package(Go "${MIN_GO_VERSION}" REQUIRED)
    if(OS_WINDOWS)
        set(ALARM_NOTIFY_GO_BIN alarm-notify.exe)
    else()
        set(ALARM_NOTIFY_GO_BIN alarm-notify)
    endif()
    add_go_target(alarm-notify-go ${ALARM_NOTIFY_GO_BIN} src/health/notifications/alarm-notify .)
endif()
