# SPDX-License-Identifier: GPL-3.0-or-later
# The static inputs the WiX MSI build consumes from the build directory.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.

include_guard()

if(OS_WINDOWS)
        configure_file(packaging/windows/netdata.wxs.in netdata.wxs @ONLY)
        configure_file(packaging/windows/NetdataWhite.ico NetdataWhite.ico COPYONLY)
        configure_file(packaging/windows/eula.rtf eula.rtf COPYONLY)
        configure_file(packaging/windows/Top.bmp Top.bmp COPYONLY)
        configure_file(packaging/windows/BackGround.bmp BackGround.bmp COPYONLY)
endif()
