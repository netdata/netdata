# SPDX-License-Identifier: GPL-3.0-or-later
# The static inputs the WiX MSI build consumes from the build directory.
#
# include()d from the root file, so paths resolve against the repository and
# build roots; nothing here may use CMAKE_CURRENT_LIST_DIR.

include_guard()

if(OS_WINDOWS)
        configure_file(packaging/windows/netdata.wxs.in netdata.wxs @ONLY)
        configure_file(packaging/windows/NetdataWhite.ico NetdataWhite.ico COPYONLY)
        configure_file(packaging/windows/eula.rtf eula.rtf COPYONLY)
        configure_file(packaging/windows/Top.bmp Top.bmp COPYONLY)
        configure_file(packaging/windows/BackGround.bmp BackGround.bmp COPYONLY)
endif()
