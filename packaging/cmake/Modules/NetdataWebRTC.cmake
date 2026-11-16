# SPDX-License-Identifier: GPL-3.0-or-later
# WebRTC dashboard communications: the bundled libdatachannel and the flags it is built with.
#
# include()d from the root file, so paths resolve against the repository and
# build roots; nothing here may use CMAKE_CURRENT_LIST_DIR.

include_guard()

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
