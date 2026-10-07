# SPDX-License-Identifier: GPL-3.0-or-later

if(NOT EXISTS "${INPUT_RC}")
    message(FATAL_ERROR "Message compiler did not produce ${INPUT_RC}")
endif()
if(NOT EXISTS "${MANIFEST_FILE}")
    message(FATAL_ERROR "Event manifest was not generated: ${MANIFEST_FILE}")
endif()

file(READ "${INPUT_RC}" _resource_contents)
file(WRITE "${OUTPUT_RC}" "${_resource_contents}\n1 2004 \"${MANIFEST_FILE}\"\n")
