# SPDX-License-Identifier: GPL-3.0-or-later
# nd-run: runs a helper program with the capabilities it needs, if any.
#
# include()d from the root file, so paths resolve against the repository and
# build roots; nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# Sets HAVE_CAPABILITY, a #cmakedefine that NetdataSystemFiles.cmake reads when it
# generates config.h on the root file's last line - so this include() must
# stay ahead of that, which its ordinal position guarantees by 50-odd lines.

include_guard()

set(NDRUN_FILES
    src/collectors/utils/nd-run.c
    src/collectors/utils/nd-process-tree.c
    src/collectors/utils/nd-process-tree.h
    src/collectors/utils/nd-file-reader.c
    src/collectors/utils/nd-file-reader.h
    src/collectors/utils/exec-signals.h)

#
# nd-run helper program
#

# libcap is Linux-only, so the OS condition IS the requirement, not an
# optimisation of the lookup.
if(CAP_FOUND AND OS_LINUX)
  set(HAVE_CAPABILITY True)
endif()

add_executable(nd-run ${NDRUN_FILES})
target_include_directories(nd-run PRIVATE ${CMAKE_BINARY_DIR})
install(TARGETS nd-run
        COMPONENT netdata
        DESTINATION "${BINDIR}")
