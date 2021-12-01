// SPDX-License-Identifier: GPL-3.0-or-later
#define NETDATA_RRD_INTERNALS

#include "metadatalog.h"

/* Return 0 on success. */
int compaction_failure_recovery(struct metalog_instance *ctx, struct metadata_logfile **metalogfiles,
                                 unsigned *matched_files)
{
    int ret;
    unsigned starting_fileno, fileno, i, j, recovered_files;
    struct metadata_logfile *metalogfile = NULL, *compactionfile = NULL, **tmp_metalogfiles;
    char *dbfiles_path = ctx->rrdeng_ctx->dbfiles_path;

    for (i = 0 ; i < *matched_files ; ++i) {
        metalogfile = metalogfiles[i];
        if (0 == metalogfile->starting_fileno)
            continue; /* skip standard metadata log files */
        break; /* this is a compaction temporary file */
    }
    if (i == *matched_files) /* no recovery needed */
        return 0;
    info("Starting metadata log file failure recovery procedure in \"%s\".", dbfiles_path);

    if (*matched_files - i > 1) { /* Can't have more than 1 temporary compaction files */
        error("Metadata log files are in an invalid state. Cannot proceed.");
        return 1;
    }
    compactionfile = metalogfile;
    starting_fileno = compactionfile->starting_fileno;
    fileno = compactionfile->fileno;
    /* scratchpad space to move file pointers around */
    tmp_metalogfiles = callocz(*matched_files, sizeof(*tmp_metalogfiles));

    for (j = 0, recovered_files = 0 ; j < i ; ++j) {
        metalogfile = metalogfiles[j];
        fatal_assert(0 == metalogfile->starting_fileno);
        if (metalogfile->fileno < starting_fileno) {
            tmp_metalogfiles[recovered_files++] = metalogfile;
            continue;
        }
        break; /* reached compaction file serial number */
    }

    if ((j == i) /* Shouldn't be possible, invalid compaction temporary file */ ||
        (metalogfile->fileno == starting_fileno && metalogfile->fileno == fileno)) {
        error("Deleting invalid compaction temporary file \"%s/"METALOG_PREFIX METALOG_FILE_NUMBER_PRINT_TMPL
              METALOG_EXTENSION"\"", dbfiles_path, starting_fileno, fileno);
        unlink_metadata_logfile(compactionfile);
        freez(compactionfile);
        freez(tmp_metalogfiles);
        --*matched_files; /* delete the last one */

        info("Finished metadata log file failure recovery procedure in \"%s\".", dbfiles_path);
        return 0;
    }

    for ( ; j < i ; ++j) { /* continue iterating through normal metadata log files */
        metalogfile = metalogfiles[j];
        fatal_assert(0 == metalogfile->starting_fileno);
        if (metalogfile->fileno < fileno) { /* It has already been compacted */
            error("Deleting invalid metadata log file \"%s/"METALOG_PREFIX METALOG_FILE_NUMBER_PRINT_TMPL
                      METALOG_EXTENSION"\"", dbfiles_path, 0U, metalogfile->fileno);
            unlink_metadata_logfile(metalogfile);
            freez(metalogfile);
            continue;
        }
        tmp_metalogfiles[recovered_files++] = metalogfile;
    }

    /* compaction temporary file is valid */
    tmp_metalogfiles[recovered_files++] = compactionfile;
    ret = rename_metadata_logfile(compactionfile, 0, starting_fileno);
    if (ret < 0) {
        error("Cannot rename temporary compaction files. Cannot proceed.");
        freez(tmp_metalogfiles);
        return 1;
    }

    memcpy(metalogfiles, tmp_metalogfiles, recovered_files * sizeof(*metalogfiles));
    *matched_files = recovered_files;
    freez(tmp_metalogfiles);

    info("Finished metadata log file failure recovery procedure in \"%s\".", dbfiles_path);
    return 0;
}
