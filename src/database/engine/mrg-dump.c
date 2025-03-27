// SPDX-License-Identifier: GPL-3.0-or-later

#include "mrg-dump.h"

// Function to unlink any existing MRG files (for cleanup)
void mrg_dump_unlink_all(void) {
    char filename[FILENAME_MAX + 1];

    // Remove the main file
    snprintfz(filename, FILENAME_MAX, "%s/" MRG_FILE_NAME, netdata_configured_cache_dir);
    unlink(filename);

    // Remove temporary file
    snprintfz(filename, FILENAME_MAX, "%s/" MRG_FILE_TMP_NAME, netdata_configured_cache_dir);
    unlink(filename);

    errno_clear();
}

// Save metrics and metadata to file
bool mrg_dump_save_all(MRG *mrg) {
    // Make sure we start with a clean slate
    mrg_dump_unlink_all();

    // Perform the save operation
    return mrg_dump_save(mrg);
}

// Load metrics and metadata from file
bool mrg_dump_load_all(MRG *mrg) {
    // Try to load from the file
    bool loaded = mrg_dump_load(mrg);

    // Clean up files after load (successful or not)
    mrg_dump_unlink_all();

    return loaded;
}

// Entry point function to be called from MRG code
bool mrg_save(MRG *mrg) {
    return mrg_dump_save_all(mrg);
}

// Entry point function to be called from MRG code
bool mrg_load(MRG *mrg) {
    return mrg_dump_load_all(mrg);
}
