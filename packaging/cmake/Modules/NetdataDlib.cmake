# SPDX-License-Identifier: GPL-3.0-or-later
# Functions and macros for handling of dlib

function(netdata_bundle_dlib)
  include(FetchContent)
  include(NetdataFetchContentExtra)

  set(FETCHCONTENT_FULLY_DISCONNECTED Off)
  set(repo https://github.com/davisking/dlib.git)
  set(tag 636c0bcd1e4f428d167699891bc12b404d2d1b41) # v19.24.8

  set(CMAKE_POLICY_DEFAULT_CMP0077 NEW)
  set(DLIB_NO_GUI_SUPPORT ON)
  set(DLIB_JPEG_SUPPORT OFF)
  set(DLIB_LINK_WITH_SQLITE3 OFF)
  set(DLIB_USE_BLAS OFF)
  set(DLIB_USE_LAPACK OFF)
  set(DLIB_USE_CUDA OFF)
  set(DLIB_PNG_SUPPORT OFF)
  set(DLIB_GIF_SUPPORT OFF)
  set(DLIB_WEBP_SUPPORT OFF)
  set(DLIB_JXL_SUPPORT OFF)
  set(DLIB_USE_MKL_FFT OFF)
  set(DLIB_USE_FFMPEG OFF)

  if(CMAKE_VERSION VERSION_GREATER_EQUAL 3.28)
    FetchContent_Declare(dlib
      GIT_REPOSITORY ${repo}
      GIT_TAG ${tag}
      CMAKE_ARGS ${NETDATA_CMAKE_PROPAGATE_TOOLCHAIN_ARGS}
      EXCLUDE_FROM_ALL
    )
  else()
    FetchContent_Declare(dlib
      GIT_REPOSITORY ${repo}
      GIT_TAG ${tag}
      CMAKE_ARGS ${NETDATA_CMAKE_PROPAGATE_TOOLCHAIN_ARGS}
    )
  endif()

  FetchContent_MakeAvailable_NoInstall(dlib)
endfunction()

function(netdata_add_dlib_to_target _target)
  get_target_property(NETDATA_DLIB_INCLUDE_DIRS dlib INTERFACE_INCLUDE_DIRECTORIES)
  target_include_directories(${_target} PRIVATE ${NETDATA_DLIB_INCLUDE_DIRS})
  target_link_libraries(${_target} PRIVATE dlib)
endfunction()
