# SPDX-License-Identifier: GPL-3.0-or-later
# Generate SQLite's amalgamation with the parser features selected by Netdata.

foreach(_required SQLITE_SOURCE_DIR SQLITE_WORK_DIR SQLITE_OUTPUT_DIR SQLITE_TCLSH SQLITE_LEMON SQLITE_MKKEYWORDHASH SQLITE_MKSOURCEID)
        if(NOT DEFINED ${_required} OR "${${_required}}" STREQUAL "")
                message(FATAL_ERROR "${_required} is required to generate the SQLite amalgamation")
        endif()
endforeach()

file(MAKE_DIRECTORY "${SQLITE_WORK_DIR}" "${SQLITE_WORK_DIR}/tsrc" "${SQLITE_OUTPUT_DIR}")
file(REMOVE_RECURSE "${SQLITE_WORK_DIR}/tsrc")
file(MAKE_DIRECTORY "${SQLITE_WORK_DIR}/tsrc")
configure_file("${SQLITE_SOURCE_DIR}/manifest" "${SQLITE_WORK_DIR}/manifest" COPYONLY)
# configure-generated SQLite feature probes are optional on Windows; the
# amalgamation uses SQLite's platform defaults when this header is empty.
file(WRITE "${SQLITE_WORK_DIR}/tsrc/sqlite_cfg.h" "")

# Mirror main.mk's SRC block, including every backslash-continued SRC += list.
file(READ "${SQLITE_SOURCE_DIR}/main.mk" _sqlite_main_mk)
string(FIND "${_sqlite_main_mk}" "\nSRC =" _sqlite_src_start)
string(FIND "${_sqlite_main_mk}" "\nTESTSRC =" _sqlite_testsrc_start)
if(_sqlite_src_start LESS 0 OR _sqlite_testsrc_start LESS 0 OR _sqlite_testsrc_start LESS _sqlite_src_start)
        message(FATAL_ERROR "Could not locate SQLite SRC block in main.mk")
endif()
math(EXPR _sqlite_src_start "${_sqlite_src_start} + 1")
math(EXPR _sqlite_src_length "${_sqlite_testsrc_start} - ${_sqlite_src_start}")
string(SUBSTRING "${_sqlite_main_mk}" ${_sqlite_src_start} ${_sqlite_src_length} _sqlite_src_block)
string(REGEX MATCHALL "\\$\\(TOP\\)/[A-Za-z0-9_./-]+" _sqlite_source_tokens "${_sqlite_src_block}")
set(_sqlite_source_paths)
foreach(_source_token IN LISTS _sqlite_source_tokens)
        string(REPLACE "$(TOP)/" "" _source_relative "${_source_token}")
        list(APPEND _sqlite_source_paths "${_source_relative}")
endforeach()

if(NOT _sqlite_source_paths)
        message(FATAL_ERROR "Could not parse SQLite main.mk source list")
endif()
foreach(_source_relative IN LISTS _sqlite_source_paths)
        if(NOT EXISTS "${SQLITE_SOURCE_DIR}/${_source_relative}")
                message(FATAL_ERROR "SQLite amalgamation source is missing: ${_source_relative}")
        endif()
        get_filename_component(_source_name "${_source_relative}" NAME)
        configure_file("${SQLITE_SOURCE_DIR}/${_source_relative}"
                "${SQLITE_WORK_DIR}/tsrc/${_source_name}" COPYONLY)
endforeach()

execute_process(COMMAND "${SQLITE_TCLSH}" "${SQLITE_SOURCE_DIR}/tool/mkctimec.tcl" "${SQLITE_WORK_DIR}/ctime.c"
        WORKING_DIRECTORY "${SQLITE_WORK_DIR}" RESULT_VARIABLE _result)
if(NOT _result EQUAL 0)
        message(FATAL_ERROR "SQLite compile-option source generation failed: ${_result}")
endif()

configure_file("${SQLITE_SOURCE_DIR}/src/parse.y" "${SQLITE_WORK_DIR}/parse.y" COPYONLY)
execute_process(COMMAND "${SQLITE_LEMON}" -DSQLITE_ENABLE_UPDATE_DELETE_LIMIT
        "-T${SQLITE_SOURCE_DIR}/tool/lempar.c" "parse.y"
        WORKING_DIRECTORY "${SQLITE_WORK_DIR}" RESULT_VARIABLE _result)
if(NOT _result EQUAL 0)
        message(FATAL_ERROR "SQLite SQL parser generation failed: ${_result}")
endif()

execute_process(COMMAND "${SQLITE_MKKEYWORDHASH}" OUTPUT_FILE "${SQLITE_WORK_DIR}/keywordhash.h"
        RESULT_VARIABLE _result)
if(NOT _result EQUAL 0)
        message(FATAL_ERROR "SQLite keyword table generation failed: ${_result}")
endif()

file(READ "${SQLITE_WORK_DIR}/parse.h" _sqlite_parse_header)
file(READ "${SQLITE_SOURCE_DIR}/src/vdbe.c" _sqlite_vdbe_source)
file(WRITE "${SQLITE_WORK_DIR}/opcodes-input.txt" "${_sqlite_parse_header}\n${_sqlite_vdbe_source}")
execute_process(COMMAND "${SQLITE_TCLSH}" "${SQLITE_SOURCE_DIR}/tool/mkopcodeh.tcl"
        INPUT_FILE "${SQLITE_WORK_DIR}/opcodes-input.txt" OUTPUT_FILE "${SQLITE_WORK_DIR}/opcodes.h"
        WORKING_DIRECTORY "${SQLITE_WORK_DIR}" RESULT_VARIABLE _result)
if(NOT _result EQUAL 0)
        message(FATAL_ERROR "SQLite opcode header generation failed: ${_result}")
endif()

execute_process(COMMAND "${SQLITE_TCLSH}" "${SQLITE_SOURCE_DIR}/tool/mkopcodec.tcl"
        "${SQLITE_WORK_DIR}/opcodes.h" OUTPUT_FILE "${SQLITE_WORK_DIR}/opcodes.c"
        WORKING_DIRECTORY "${SQLITE_WORK_DIR}" RESULT_VARIABLE _result)
if(NOT _result EQUAL 0)
        message(FATAL_ERROR "SQLite opcode source generation failed: ${_result}")
endif()

execute_process(COMMAND "${SQLITE_TCLSH}" "${SQLITE_SOURCE_DIR}/tool/mkpragmatab.tcl"
        "${SQLITE_WORK_DIR}/pragma.h" WORKING_DIRECTORY "${SQLITE_WORK_DIR}" RESULT_VARIABLE _result)
if(NOT _result EQUAL 0)
        message(FATAL_ERROR "SQLite pragma table generation failed: ${_result}")
endif()

configure_file("${SQLITE_SOURCE_DIR}/ext/fts5/fts5parse.y" "${SQLITE_WORK_DIR}/fts5parse.y" COPYONLY)
execute_process(COMMAND "${SQLITE_LEMON}" "-T${SQLITE_SOURCE_DIR}/tool/lempar.c" "fts5parse.y"
        WORKING_DIRECTORY "${SQLITE_WORK_DIR}" RESULT_VARIABLE _result)
if(NOT _result EQUAL 0)
        message(FATAL_ERROR "SQLite FTS5 parser generation failed: ${_result}")
endif()
execute_process(COMMAND "${SQLITE_TCLSH}" "${SQLITE_SOURCE_DIR}/ext/fts5/tool/mkfts5c.tcl"
        WORKING_DIRECTORY "${SQLITE_WORK_DIR}" RESULT_VARIABLE _result)
if(NOT _result EQUAL 0)
        message(FATAL_ERROR "SQLite FTS5 source generation failed: ${_result}")
endif()
configure_file("${SQLITE_SOURCE_DIR}/ext/fts5/fts5.h" "${SQLITE_WORK_DIR}/tsrc/fts5.h" COPYONLY)

set(_sqlite_mksourceid_in_work "${SQLITE_WORK_DIR}/mksourceid.exe")
file(REMOVE "${_sqlite_mksourceid_in_work}")
configure_file("${SQLITE_MKSOURCEID}" "${_sqlite_mksourceid_in_work}" COPYONLY)
execute_process(COMMAND "${SQLITE_TCLSH}" "${SQLITE_SOURCE_DIR}/tool/mksqlite3h.tcl"
        "${SQLITE_SOURCE_DIR}" -o "${SQLITE_WORK_DIR}/sqlite3.h"
        WORKING_DIRECTORY "${SQLITE_WORK_DIR}" RESULT_VARIABLE _result)
file(REMOVE "${_sqlite_mksourceid_in_work}")
if(NOT _result EQUAL 0)
        message(FATAL_ERROR "SQLite public header generation failed: ${_result}")
endif()

foreach(_generated keywordhash.h opcodes.c opcodes.h parse.c parse.h pragma.h sqlite3.h)
        configure_file("${SQLITE_WORK_DIR}/${_generated}"
                "${SQLITE_WORK_DIR}/tsrc/${_generated}" COPYONLY)
endforeach()
configure_file("${SQLITE_WORK_DIR}/ctime.c" "${SQLITE_WORK_DIR}/tsrc/ctime.c" COPYONLY)
configure_file("${SQLITE_WORK_DIR}/fts5.c" "${SQLITE_WORK_DIR}/tsrc/fts5.c" COPYONLY)

execute_process(COMMAND "${SQLITE_TCLSH}" "${SQLITE_SOURCE_DIR}/tool/mksqlite3c.tcl"
        --srcdir "${SQLITE_WORK_DIR}/tsrc"
        WORKING_DIRECTORY "${SQLITE_WORK_DIR}" RESULT_VARIABLE _result)
if(NOT _result EQUAL 0)
        message(FATAL_ERROR "SQLite amalgamation generation failed: ${_result}")
endif()

foreach(_file sqlite3.c sqlite3.h)
        configure_file("${SQLITE_WORK_DIR}/${_file}" "${SQLITE_OUTPUT_DIR}/${_file}" COPYONLY)
endforeach()
foreach(_file sqlite3recover.c sqlite3recover.h dbdata.c)
        configure_file("${SQLITE_SOURCE_DIR}/ext/recover/${_file}"
                "${SQLITE_OUTPUT_DIR}/${_file}" COPYONLY)
endforeach()
