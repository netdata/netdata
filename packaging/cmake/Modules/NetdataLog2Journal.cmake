# SPDX-License-Identifier: GPL-3.0-or-later
# log2journal: converts structured text logs into systemd Journal Export Format.
#
# Relocated from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# One entry was later dropped rather than moved: LOG2JOURNAL_FILES opened with
# ${CONFIG_H}, naming the generated config.h. A header in a source list produces
# no build edge and no compile flag, so it went with the variable itself.
#
# Built only when libpcre2-8 was found. The lookup lives in NetdataDependencies, and
# log2journal is optional, so this reads PCRE2_FOUND instead of requiring it.

include_guard()

# log2journal
set(LOG2JOURNAL_FILES
        src/collectors/log2journal/log2journal.h
        src/collectors/log2journal/log2journal.c
        src/collectors/log2journal/log2journal-help.c
        src/collectors/log2journal/log2journal-yaml.c
        src/collectors/log2journal/log2journal-json.c
        src/collectors/log2journal/log2journal-logfmt.c
        src/collectors/log2journal/log2journal-pcre2.c
        src/collectors/log2journal/log2journal-params.c
        src/collectors/log2journal/log2journal-inject.c
        src/collectors/log2journal/log2journal-pattern.c
        src/collectors/log2journal/log2journal-replace.c
        src/collectors/log2journal/log2journal-rename.c
        src/collectors/log2journal/log2journal-rewrite.c
        src/collectors/log2journal/log2journal-txt.h
        src/collectors/log2journal/log2journal-hashed-key.h
)

#
# build log2journal
#

if(PCRE2_FOUND)
        add_executable(log2journal ${LOG2JOURNAL_FILES})
        target_include_directories(log2journal BEFORE PUBLIC ${CONFIG_H_DIR} ${CMAKE_SOURCE_DIR}/src ${PCRE2_INCLUDE_DIRS})
        target_compile_options(log2journal PUBLIC ${PCRE2_CFLAGS_OTHER})

        target_link_libraries(log2journal PUBLIC libnetdata)
        target_link_libraries(log2journal PUBLIC "${PCRE2_LDFLAGS}")
        netdata_add_libyaml_to_target(log2journal)

        install(TARGETS log2journal
                COMPONENT netdata
                DESTINATION "${BINDIR}")

        install(DIRECTORY src/collectors/log2journal/log2journal.d
                COMPONENT netdata
                DESTINATION ${LIBCONFIG_DEST})
endif()
