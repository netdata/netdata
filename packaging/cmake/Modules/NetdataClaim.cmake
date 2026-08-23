# SPDX-License-Identifier: GPL-3.0-or-later
# NetdataClaim: the Windows claiming helper, shipped through the MSI component path.
#
# include()d from the root file, so paths resolve against the repository and
# build roots; nothing here may use CMAKE_CURRENT_LIST_DIR.

include_guard()

set(CLAIM_WINDOWS_FILES
        src/claim/main.c
        src/claim/main.h
        src/claim/ui.c
        src/claim/ui.h
)

# The claim helper's resource script and its manifest.
set(NETDATA_CLAIM_RES_FILES "packaging/windows/resources/netdata_claim.rc")

if(OS_WINDOWS)
        configure_file(packaging/windows/resources/netdata_claim.manifest.in ${CMAKE_SOURCE_DIR}/packaging/windows/resources/netdata_claim.manifest @ONLY)

        add_executable(NetdataClaim ${CLAIM_WINDOWS_FILES} ${NETDATA_CLAIM_RES_FILES})
        target_link_libraries(NetdataClaim shell32 gdi32 msftedit)
        target_compile_options(NetdataClaim PUBLIC -mwindows)
endif()
