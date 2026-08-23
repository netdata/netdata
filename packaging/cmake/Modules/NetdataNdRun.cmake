# SPDX-License-Identifier: GPL-3.0-or-later
# nd-run: runs a helper program with the capabilities it needs, if any.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# Sets HAVE_CAPABILITY, a #cmakedefine that NetdataSystemFiles reads when it
# generates config.h on the root file's last line - so this include() must
# stay ahead of that, which its ordinal position guarantees by 400-odd lines.

include_guard()

set(NDRUN_FILES src/collectors/utils/nd-run.c)

#
# nd-run helper program
#

# libcap is Linux-only, so the OS condition IS the requirement, not an
# optimisation of the lookup.
if(CAP_FOUND AND OS_LINUX)
  set(HAVE_CAPABILITY True)
endif()

add_executable(nd-run ${NDRUN_FILES})
if(HAVE_CAPABILITY)
  target_link_libraries(nd-run PRIVATE PkgConfig::CAP)
endif()
target_include_directories(nd-run PRIVATE ${CMAKE_BINARY_DIR})
install(TARGETS nd-run
        COMPONENT netdata
        DESTINATION "${BINDIR}")
