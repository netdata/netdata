# SPDX-License-Identifier: GPL-3.0-or-later
# systemd-journal: journal log collection, with the Rust reader fallback.
#
# include()d from the root file, so paths resolve against the repository and
# build roots; nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# The two blocks move together: the first decides
# ENABLE_NETDATA_JOURNAL_FILE_READER and the second is its only consumer. Both
# keep their original position relative to each other and to the rest of the
# file.
#
# The source list lives here rather than in the shared
# NetdataSourceLists.cmake, so a plugin's whole definition is in one place. It
# sits above the guards, so it is defined unconditionally.
#
# Known defect, pre-existing and deliberately left alone here: the fallback
# below sets ENABLE_NETDATA_JOURNAL_FILE_READER too late to matter. The root
# file decides whether to import the journal_reader_ffi crate long before
# detect_systemd() runs, so SYSTEMD_FOUND is not yet known at that point. On a
# host with the journal plugin enabled but no libsystemd, the crate is never
# imported and this module then links a target that was never created. Fixing
# it means reordering the detection, which belongs in a change where the
# journal reader is the subject.

include_guard()

set(SYSTEMD_JOURNAL_PLUGIN_FILES
        src/collectors/systemd-journal.plugin/systemd-journal-fstat.c
        src/collectors/systemd-journal.plugin/systemd-internals.h
        src/collectors/systemd-journal.plugin/systemd-main.c
        src/collectors/systemd-journal.plugin/systemd-journal.c
        src/collectors/systemd-journal.plugin/systemd-journal-function.h
        src/collectors/systemd-journal.plugin/systemd-journal-execute.h
        src/collectors/systemd-journal.plugin/systemd-journal-annotations.c
        src/collectors/systemd-journal.plugin/systemd-journal-files.c
        src/collectors/systemd-journal.plugin/systemd-journal-watcher.c
        src/collectors/systemd-journal.plugin/systemd-journal-dyncfg.c
        src/collectors/systemd-journal.plugin/provider/netdata_provider.c
        src/collectors/systemd-journal.plugin/provider/netdata_provider.h
        src/collectors/systemd-journal.plugin/provider/rust_provider.h
        src/libnetdata/os/system-maps/system-services.h
        src/collectors/systemd-journal.plugin/systemd-journal-sampling.h
)

# Enable rust implementation if we don't have systemd and we want the journal plugin
if(ENABLE_PLUGIN_SYSTEMD_JOURNAL AND NOT SYSTEMD_FOUND)
        if (NOT ENABLE_NETDATA_JOURNAL_FILE_READER)
                message(WARNING "Systemd journal package not found, will try netdata's journal reader which requires cargo.")
                set(ENABLE_NETDATA_JOURNAL_FILE_READER True)
        endif()
endif()

if(ENABLE_PLUGIN_SYSTEMD_JOURNAL)
        add_executable(systemd-journal.plugin ${SYSTEMD_JOURNAL_PLUGIN_FILES})

        if(ENABLE_NETDATA_JOURNAL_FILE_READER)
                target_compile_definitions(systemd-journal.plugin PRIVATE HAVE_RUST_PROVIDER)
                target_link_libraries(systemd-journal.plugin journal_reader_ffi)
        endif()

        target_link_libraries(systemd-journal.plugin libnetdata)

        install(TARGETS systemd-journal.plugin
                COMPONENT plugin-systemd-journal
                DESTINATION ${PLUGINS_DEST})

        netdata_add_deb_copyright(plugin-systemd-journal netdata-plugin-systemd-journal)
endif()
