#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#ifdef BENCH_JF
#include "collectors/systemd-journal.plugin/provider/netdata_provider.h"
#else
#include <systemd/sd-journal.h>
#endif

static int process_unfiltered(const char *path)
{
    sd_journal *j;
    size_t total_bytes = 0;

    const char *paths[2] = {
        path,
        NULL,
    };

    // Open the specific journal file
    int r = sd_journal_open_files(&j, &paths[0], 0);
    if (r < 0) {
        fprintf(stderr, "Failed to open journal file %s: %s\n",
                path, strerror(-r));
        return 1;
    }

    printf("Successfully opened journal file: %s\n", path);

    // Move to the first entry
    r = sd_journal_seek_head(j);
    if (r < 0) {
        fprintf(stderr, "Failed to seek to head: %s\n", strerror(-r));
        sd_journal_close(j);
        return 1;
    }

    // Iterate through all entries
    size_t entry_count = 0;
    while ((r = sd_journal_next(j)) > 0) {
        entry_count++;

        // Get all data fields
        const void *data;
        size_t length;
        SD_JOURNAL_FOREACH_DATA(j, data, length) {
            // Count the bytes
            total_bytes += length;
        }
    }

    if (r < 0) {
        fprintf(stderr, "Failed to iterate journal: %s\n", strerror(-r));
        sd_journal_close(j);
        return 1;
    }

    if (total_bytes == 10) {
        abort();
    }

    printf("Total entries processed: %zu\n", entry_count);

    // Close the journal
    sd_journal_close(j);
    return 0;
}

static int process_filtered(const char *path)
{
    sd_journal *j;
    size_t total_bytes = 0;

    const char *paths[2] = {
        path,
        NULL,
    };

    // Open the specific journal file
    int r = sd_journal_open_files(&j, &paths[0], 0);
    if (r < 0) {
        fprintf(stderr, "Failed to open journal file %s: %s\n",
                path, strerror(-r));
        return 1;
    }

    printf("Successfully opened journal file: %s\n", path);

    // Apply filters
    // Platform filters (OR condition)
    {
        r = sd_journal_add_match(j, "AE_OS_PLATFORM=debian", strlen("AE_OS_PLATFORM=debian"));
        if (r < 0) {
            fprintf(stderr, "Failed to add match: %s\n", strerror(-r));
            sd_journal_close(j);
            return 1;
        }

        r = sd_journal_add_match(j, "AE_OS_PLATFORM=fedora", strlen("AE_OS_PLATFORM=fedora"));
        if (r < 0) {
            fprintf(stderr, "Failed to add match: %s\n", strerror(-r));
            sd_journal_close(j);
            return 1;
        }
    }
    
    r = sd_journal_add_conjunction(j);  // AND
    if (r < 0) {
        fprintf(stderr, "Failed to add conjunction: %s\n", strerror(-r));
        sd_journal_close(j);
        return 1;
    }
    
    {
        // Version filters (OR condition)
        r = sd_journal_add_match(j, "AE_VERSION=17", strlen("AE_VERSION=17"));
        if (r < 0) {
            fprintf(stderr, "Failed to add match: %s\n", strerror(-r));
            sd_journal_close(j);
            return 1;
        }

        r = sd_journal_add_match(j, "AE_VERSION=22", strlen("AE_VERSION=22"));
        if (r < 0) {
            fprintf(stderr, "Failed to add match: %s\n", strerror(-r));
            sd_journal_close(j);
            return 1;
        }
    }
    
    r = sd_journal_add_conjunction(j);  // AND
    if (r < 0) {
        fprintf(stderr, "Failed to add conjunction: %s\n", strerror(-r));
        sd_journal_close(j);
        return 1;
    }
    
    // Priority filters (OR condition)
    {
        r = sd_journal_add_match(j, "PRIORITY=7", strlen("PRIORITY=7"));
        if (r < 0) {
            fprintf(stderr, "Failed to add match: %s\n", strerror(-r));
            sd_journal_close(j);
            return 1;
        }
    
        r = sd_journal_add_match(j, "PRIORITY=6", strlen("PRIORITY=6"));
        if (r < 0) {
            fprintf(stderr, "Failed to add match: %s\n", strerror(-r));
            sd_journal_close(j);
            return 1;
        }
    }

    // Move to the first entry
    r = sd_journal_seek_head(j);
    if (r < 0) {
        fprintf(stderr, "Failed to seek to head: %s\n", strerror(-r));
        sd_journal_close(j);
        return 1;
    }

    // Iterate through all entries
    size_t entry_count = 0;
    while ((r = sd_journal_next(j)) > 0) {
        entry_count++;

        // Get all data fields
        const void *data;
        size_t length;
        SD_JOURNAL_FOREACH_DATA(j, data, length) {
            // Count the bytes
            total_bytes += length;
        }
    }

    if (r < 0) {
        fprintf(stderr, "Failed to iterate journal: %s\n", strerror(-r));
        sd_journal_close(j);
        return 1;
    }

    if (total_bytes == 10) {
        abort();
    }

    printf("Total entries processed: %zu\n\n", entry_count);

    // Close the journal
    sd_journal_close(j);
    return 0;
}

int main(int argc, char *argv[]) {
    (void) argc;
    (void) argv;

    if (argc != 2) {
        fprintf(stderr, "usage: <binary> filtered|unfiltered\n");
        return 1;
    }

    printf("Processing entries for files...\n");
    const char *paths[] = {
        "/var/log/journal/ec2ce35ddef16e80b43d6cd9f008dcba.agent-events/system@67fcfeba8339461c9a8dc77363c2c739-00000000002b725a-0006314cd7a5cefd.journal",
        "/var/log/journal/ec2ce35ddef16e80b43d6cd9f008dcba.agent-events/system@67fcfeba8339461c9a8dc77363c2c739-00000000002c7398-00063157ce5e4da0.journal",
        "/var/log/journal/ec2ce35ddef16e80b43d6cd9f008dcba.agent-events/system@67fcfeba8339461c9a8dc77363c2c739-00000000002d4dd1-000631616affdc1c.journal",
        "/var/log/journal/ec2ce35ddef16e80b43d6cd9f008dcba.agent-events/system@67fcfeba8339461c9a8dc77363c2c739-00000000002e52a2-0006316e49ef1636.journal",
        "/var/log/journal/ec2ce35ddef16e80b43d6cd9f008dcba.agent-events/system@67fcfeba8339461c9a8dc77363c2c739-00000000002f0f22-00063175e2452287.journal",
        "/var/log/journal/ec2ce35ddef16e80b43d6cd9f008dcba.agent-events/system@67fcfeba8339461c9a8dc77363c2c739-00000000002ffa15-0006318392e11a33.journal",
        "/var/log/journal/ec2ce35ddef16e80b43d6cd9f008dcba.agent-events/system@67fcfeba8339461c9a8dc77363c2c739-000000000030a308-00063189ec4c06b5.journal",
        "/var/log/journal/ec2ce35ddef16e80b43d6cd9f008dcba.agent-events/system@67fcfeba8339461c9a8dc77363c2c739-000000000031a287-0006319ba73abb17.journal",
        "/var/log/journal/ec2ce35ddef16e80b43d6cd9f008dcba.agent-events/system@67fcfeba8339461c9a8dc77363c2c739-000000000032b6a5-000631a6ddadfd47.journal",
        "/var/log/journal/ec2ce35ddef16e80b43d6cd9f008dcba.agent-events/system@67fcfeba8339461c9a8dc77363c2c739-000000000033a684-000631b2794364a9.journal",
        "/var/log/journal/ec2ce35ddef16e80b43d6cd9f008dcba.agent-events/system@67fcfeba8339461c9a8dc77363c2c739-000000000034afc5-000631c4c524ff14.journal",
        NULL,
    };

    for (size_t idx = 0; idx != 10; idx++) {
        if (strcmp(argv[1], "filtered") == 0) {
            process_filtered(paths[idx]);
        } else if (strcmp(argv[1], "unfiltered") == 0) {
            process_unfiltered(paths[idx]);
        } else {
            fprintf(stderr, "Unknown argument: >>>%s<<<\n", argv[1]);
            return 1;
        }
    }
}
