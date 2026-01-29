# SPDX-License-Identifier: GPL-3.0-or-later
# CMake Module to handle all the systemd-related checks for Netdata.

include(CMakePushCheckState)

macro(detect_systemd)
  find_library(SYSTEMD_LIBRARY NAMES systemd)

  set(ENABLE_DSYSTEMD_DBUS NO)
  pkg_check_modules(SYSTEMD libsystemd)

  if(SYSTEMD_FOUND)
    cmake_push_check_state()
    set(CMAKE_REQUIRED_LIBRARIES "${CMAKE_REQUIRED_LIBRARIES};${SYSTEMD_LIBRARIES}")

    check_c_source_compiles("
    #include <systemd/sd-journal.h>

    int main() {
      int x = SD_JOURNAL_OS_ROOT;
      return 0;
    }" HAVE_SD_JOURNAL_OS_ROOT)

    check_symbol_exists(SD_JOURNAL_OS_ROOT "systemd/sd-journal.h" HAVE_SD_JOURNAL_OS_ROOT)
    check_symbol_exists(sd_journal_open_files_fd "systemd/sd-journal.h" HAVE_SD_JOURNAL_OPEN_FILES_FD)
    check_symbol_exists(sd_journal_restart_fields "systemd/sd-journal.h" HAVE_SD_JOURNAL_RESTART_FIELDS)
    check_symbol_exists(sd_journal_get_seqnum "systemd/sd-journal.h" HAVE_SD_JOURNAL_GET_SEQNUM)

    check_symbol_exists(sd_bus_default_system "systemd/sd-bus.h" HAVE_SD_BUS_DEFAULT_SYSTEM)
    check_symbol_exists(sd_bus_call_method "systemd/sd-bus.h" HAVE_SD_BUS_CALL_METHOD)
    check_symbol_exists(sd_bus_message_enter_container "systemd/sd-bus.h" HAVE_SD_BUS_MESSAGE_ENTER_CONTAINER)
    check_symbol_exists(sd_bus_message_read "systemd/sd-bus.h" HAVE_SD_BUS_MESSAGE_READ)
    check_symbol_exists(sd_bus_message_exit_container "systemd/sd-bus.h" HAVE_SD_BUS_MESSAGE_EXIT_CONTAINER)

    cmake_pop_check_state()
    set(HAVE_SYSTEMD True)
    if(HAVE_SD_BUS_DEFAULT_SYSTEM AND HAVE_SD_BUS_CALL_METHOD AND HAVE_SD_BUS_MESSAGE_ENTER_CONTAINER AND HAVE_SD_BUS_MESSAGE_READ AND HAVE_SD_BUS_MESSAGE_EXIT_CONTAINER)
      set(ENABLE_SYSTEMD_DBUS YES)
    endif()
  endif()
endmacro()
