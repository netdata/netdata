# SPDX-License-Identifier: GPL-3.0-or-later
#
# Platform detection code.
#
# This sets various OS_* and CPU_* variables based on the OS.
#
# This sorts out what OS and CPU we’re building for, which is
# information used by numerous other parts of the build system.

include_guard()

macro(_nd_windows_config)
  set(OS_WINDOWS True)

  # On Linux and the MSYS-runtime cmake, the prefix stays verbatim as
  # /opt/netdata. The UCRT64 cmake is a native Windows binary and
  # canonicalises /opt/netdata to its MSYS2-mounted form (typically
  # C:/msys64/opt/netdata). Both refer to the same on-disk location, so
  # accept either: the exact string, or anything ending in /opt/netdata.
  if(NOT "${CMAKE_INSTALL_PREFIX}" STREQUAL "/opt/netdata" AND
     NOT "${CMAKE_INSTALL_PREFIX}" MATCHES "/opt/netdata$")
    message(FATAL_ERROR "CMAKE_INSTALL_PREFIX must be set to /opt/netdata, but it is set to ${CMAKE_INSTALL_PREFIX}")
  endif()

  # The Windows build only supports the MSYS2 UCRT64 environment. Other
  # MSYS2 environments (MSYS, MINGW64, CLANG64) are deliberately rejected
  # to keep the C runtime, data model, and CRT linkage consistent with the
  # Rust toolchain (which is also UCRT-linked). See SOW-0033.
  if(NOT "$ENV{MSYSTEM}" STREQUAL "UCRT64")
    message(FATAL_ERROR
      "The Windows build requires MSYSTEM=UCRT64. "
      "Current MSYSTEM='$ENV{MSYSTEM}'. "
      "Launch the MSYS2 UCRT64 shell (or set MSYSTEM=UCRT64 before invoking bash).")
  endif()

  if(BUILD_FOR_PACKAGING)
    set(NETDATA_RUNTIME_PREFIX "/")
  endif()

  set(BINDIR usr/bin)
  set(CMAKE_RC_COMPILER_INIT windres)
  ENABLE_LANGUAGE(RC)

  SET(CMAKE_RC_COMPILE_OBJECT "<CMAKE_RC_COMPILER> <FLAGS> -O coff <DEFINES> -i <SOURCE> -o <OBJECT>")
  add_definitions(-D_GNU_SOURCE)

  if($ENV{CLION_IDE})
    set(RUN_UNDER_CLION True)
    include_directories(c:/msys64/ucrt64/include)
  endif()

  message(STATUS " Compiling for Windows (${CMAKE_SYSTEM_NAME}, MSYSTEM=$ENV{MSYSTEM})... ")
endmacro()

set(OS_FREEBSD     False)
set(OS_LINUX       False)
set(OS_MACOS       False)
set(OS_WINDOWS     False)

set(CPU_X86     False)
set(CPU_X86_64  False)
set(CPU_ARM     False)
set(CPU_ARM64   False)
set(CPU_OTHER   False)

if("${CMAKE_SYSTEM_NAME}" STREQUAL "Darwin")
  set(OS_MACOS True)
  find_library(IOKIT IOKit)
  find_library(FOUNDATION Foundation)
  message(STATUS " Compiling for MacOS... ")
elseif("${CMAKE_SYSTEM_NAME}" STREQUAL "FreeBSD")
  set(OS_FREEBSD True)
  message(STATUS " Compiling for FreeBSD... ")
elseif("${CMAKE_SYSTEM_NAME}" STREQUAL "Linux")
  set(OS_LINUX True)
  add_definitions(-D_GNU_SOURCE)
  message(STATUS " Compiling for Linux... ")
elseif("${CMAKE_SYSTEM_NAME}" STREQUAL "Windows")
  _nd_windows_config()
else()
  message(FATAL_ERROR "Unknown/unsupported platform: ${CMAKE_SYSTEM_NAME} (Supported platforms: FreeBSD, Linux, macOS, Windows)")
endif()

string(STRIP "${CMAKE_SYSTEM_PROCESSOR}" _nd_cpu_raw)
string(TOLOWER "${_nd_cpu_raw}" _nd_cpu)

message(STATUS "Detected CPU: ${CMAKE_SYSTEM_PROCESSOR}")

if("${_nd_cpu}" MATCHES "^(x86_64|x64|amd64)")
  set(CPU_X86_64 True)
elseif("${_nd_cpu}" MATCHES "^(x86|i[3456]86|ix86)")
  set(CPU_X86 True)
elseif("${_nd_cpu}" MATCHES "^(aarch|arm)64")
  set(CPU_ARM64 True)
elseif("${_nd_cpu}" MATCHES "^(arm|armv[567].*)")
  set(CPU_ARM True)
else()
  set(CPU_OTHER True)
endif()
