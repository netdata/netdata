# SPDX-License-Identifier: GPL-3.0-or-later
# WebRTC dashboard communications: the bundled libdatachannel and the flags it is built with.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.

if(ENABLE_WEBRTC)
        include(FetchContent)
        include(NetdataFetchContentExtra)

        set(PREFER_SYSTEM_LIB True)
        set(NO_MEDIA True)
        set(NO_WEBSOCKET True)

        set(HAVE_LIBDATACHANNEL True)

        FetchContent_Declare(libdatachannel
            GIT_REPOSITORY https://github.com/paullouisageneau/libdatachannel.git
            GIT_TAG v0.20.1
        )
        FetchContent_MakeAvailable(libdatachannel)
endif()
