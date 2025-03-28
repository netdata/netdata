// SPDX-License-Identifier: GPL-3.0-or-later

#include "mrg-dump.h"

// Save metrics and metadata to file
bool mrg_dump_save_all(MRG *mrg) {
    // Perform the save operation
    return mrg_dump_save(mrg);
}

// Load metrics and metadata from file
bool mrg_dump_load_all(MRG *mrg) {
    // Try to load from the file
    bool loaded = mrg_dump_load(mrg);

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
