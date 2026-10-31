# SPDX-License-Identifier: GPL-3.0-or-later

execute_process(COMMAND "${WEVT_GENERATOR}"
        OUTPUT_FILE "${WEVT_MC_FILE}"
        RESULT_VARIABLE _mc_result)
if(NOT _mc_result EQUAL 0)
    message(FATAL_ERROR "Failed to generate the Windows message compiler input")
endif()

execute_process(COMMAND "${WEVT_GENERATOR}" --manifest
        OUTPUT_FILE "${WEVT_MAN_FILE}"
        RESULT_VARIABLE _manifest_result)
if(NOT _manifest_result EQUAL 0)
    message(FATAL_ERROR "Failed to generate the Windows ETW manifest")
endif()
