# SPDX-License-Identifier: GPL-3.0-or-later
# netdata_driver: the Windows kernel driver, built as a .sys with the MinGW DDK.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.

include_guard()

set(WINDOWS_DRIVER_FILES
        src/collectors/windows.plugin/driver/netdata_driver.c
        src/collectors/windows.plugin/driver/netdata_driver.h
)

if(OS_WINDOWS)
        configure_file(src/collectors/windows.plugin/driver/netdata_driver.inf netdata_driver.inf COPYONLY)

        set(NETDATA_DRIVER_FILE "${CMAKE_BINARY_DIR}/netdata_driver.sys")
        set(NETDATA_DRIVER_FILE_INF "${CMAKE_BINARY_DIR}/netdata_driver.inf")

        add_library(netdata_driver SHARED ${WINDOWS_DRIVER_FILES})
        set_target_properties(netdata_driver PROPERTIES LIBRARY_OUTPUT_NAME "netdata_driver")
        set_target_properties(netdata_driver PROPERTIES PREFIX "")
        set_target_properties(netdata_driver PROPERTIES SUFFIX ".sys")
        target_include_directories(netdata_driver PRIVATE BEFORE "/mingw64/include/ddk" "${CMAKE_SOURCE_DIR}/src/collectors/windows.plugin" "${CMAKE_SOURCE_DIR}/src/collectors/windows.plugin/driver")
        target_compile_options(netdata_driver PRIVATE
                -Wall
                -Wextra
                -Werror
                -ffreestanding
                -fno-exceptions
                -fno-stack-protector
                -fshort-wchar
                -mno-red-zone
        )
        target_link_options(netdata_driver PRIVATE
                -Wl,--entry,DriverEntry@8
                -nostdlib
                -Wl,--subsystem,native
                -Wl,--image-base,0x10000
        )
        target_link_libraries(netdata_driver kernel32 ntoskrnl)

        install(FILES "${NETDATA_DRIVER_FILE}"
                COMPONENT netdata_driver
                DESTINATION "${BINDIR}")

        install(FILES "${NETDATA_DRIVER_FILE_INF}"
                COMPONENT netdata_driver_inf
                DESTINATION "${BINDIR}")
endif()
