#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "collectors/systemd-journal.plugin/provider/netdata_provider.h"

void format_entry(NsdJournal *j, size_t entry_id) {
    size_t data_count = 0;
    char buf[4096];

    const void *data;
    size_t length;
    NSD_JOURNAL_FOREACH_DATA(j, data, length) {
        if (length > 4095) {
            fprintf(stderr, "length is more than 4KiB\n");
            nsd_journal_close(j);
            exit(EXIT_FAILURE);
        }

        memcpy(buf, data, length);
        buf[length] = '\0';

        fprintf(stdout, "E[%zu] D[%zu] %s\n", entry_id, data_count, buf);
        data_count += 1;
    }
}

static int process_unfiltered(const char *path)
{
    NsdJournal *j;
    size_t total_bytes = 0;

    const char *paths[2] = {
        path,
        NULL,
    };

    // Open the specific journal file
    int r = nsd_journal_open_files(&j, &paths[0], 0);
    if (r < 0) {
        fprintf(stderr, "Failed to open journal file %s: %s\n",
                path, strerror(-r));
        return 1;
    }

    printf("Successfully opened journal file: %s\n", path);

    {
        const char *field = NULL;
        nsd_journal_restart_fields(j);

        // Enumerate all field names in the journal
        while (nsd_journal_enumerate_fields(j, &field) > 0) {
            printf("Field name: %s\n", field);
        }
    }

    // Move to the first entry
    r = nsd_journal_seek_head(j);
    if (r < 0) {
        fprintf(stderr, "Failed to seek to head: %s\n", strerror(-r));
        nsd_journal_close(j);
        return 1;
    }

    // Iterate through all entries
    size_t entry_count = 0;
    while ((r = nsd_journal_next(j)) > 0) {
        entry_count++;

        // Get all data fields
        const void *data;
        size_t length;
        NSD_JOURNAL_FOREACH_DATA(j, data, length) {
            // Count the bytes
            total_bytes += length;
        }
    }

    if (r < 0) {
        fprintf(stderr, "Failed to iterate journal: %s\n", strerror(-r));
        nsd_journal_close(j);
        return 1;
    }

    if (total_bytes == 10) {
        abort();
    }

    printf("Total entries processed: %zu\n", entry_count);

    // Close the journal
    nsd_journal_close(j);
    return 0;
}

static int process_filtered(const char *path)
{
    NsdJournal *j;
    size_t total_bytes = 0;

    const char *paths[2] = {
        path,
        NULL,
    };

    // Open the specific journal file
    int r = nsd_journal_open_files(&j, &paths[0], 0);
    if (r < 0) {
        fprintf(stderr, "Failed to open journal file %s: %s\n",
                path, strerror(-r));
        return 1;
    }

    printf("Successfully opened journal file: %s\n", path);

    // Apply filters
    // Platform filters (OR condition)
    {
        r = nsd_journal_add_match(j, "AE_OS_PLATFORM=debian", strlen("AE_OS_PLATFORM=debian"));
        if (r < 0) {
            fprintf(stderr, "Failed to add match: %s\n", strerror(-r));
            nsd_journal_close(j);
            return 1;
        }

        r = nsd_journal_add_match(j, "AE_OS_PLATFORM=fedora", strlen("AE_OS_PLATFORM=fedora"));
        if (r < 0) {
            fprintf(stderr, "Failed to add match: %s\n", strerror(-r));
            nsd_journal_close(j);
            return 1;
        }
    }
    
    r = nsd_journal_add_conjunction(j);  // AND
    if (r < 0) {
        fprintf(stderr, "Failed to add conjunction: %s\n", strerror(-r));
        nsd_journal_close(j);
        return 1;
    }
    
    {
        // Version filters (OR condition)
        r = nsd_journal_add_match(j, "AE_VERSION=17", strlen("AE_VERSION=17"));
        if (r < 0) {
            fprintf(stderr, "Failed to add match: %s\n", strerror(-r));
            nsd_journal_close(j);
            return 1;
        }

        r = nsd_journal_add_match(j, "AE_VERSION=22", strlen("AE_VERSION=22"));
        if (r < 0) {
            fprintf(stderr, "Failed to add match: %s\n", strerror(-r));
            nsd_journal_close(j);
            return 1;
        }
    }
    
    r = nsd_journal_add_conjunction(j);  // AND
    if (r < 0) {
        fprintf(stderr, "Failed to add conjunction: %s\n", strerror(-r));
        nsd_journal_close(j);
        return 1;
    }
    
    // Priority filters (OR condition)
    {
        r = nsd_journal_add_match(j, "PRIORITY=7", strlen("PRIORITY=7"));
        if (r < 0) {
            fprintf(stderr, "Failed to add match: %s\n", strerror(-r));
            nsd_journal_close(j);
            return 1;
        }
    
        r = nsd_journal_add_match(j, "PRIORITY=6", strlen("PRIORITY=6"));
        if (r < 0) {
            fprintf(stderr, "Failed to add match: %s\n", strerror(-r));
            nsd_journal_close(j);
            return 1;
        }
    }

    r = nsd_journal_seek_tail(j);
    if (r < 0) {
        fprintf(stderr, "Failed to seek to head: %s\n", strerror(-r));
        nsd_journal_close(j);
        return 1;
    }

    size_t entry_count = 0;
    while ((r = nsd_journal_previous(j)) > 0) {
        format_entry(j, entry_count);

        entry_count++;
    }

    if (r < 0) {
        fprintf(stderr, "Failed to iterate journal: %s\n", strerror(-r));
        nsd_journal_close(j);
        return 1;
    }

    if (total_bytes == 10) {
        abort();
    }

    printf("Total entries processed: %zu\n\n", entry_count);

    // Close the journal
    nsd_journal_close(j);
    return 0;
}

long get_file_size(const char *filename) {
    FILE *file = fopen(filename, "rb");
    
    if (file == NULL) {
        abort();
    }
    
    // Seek to the end of the file
    fseek(file, 0, SEEK_END);
    
    // Get the current position (which is the size)
    long size = ftell(file);
    
    // Close the file
    fclose(file);
    
    return size;
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

    size_t total_size = 0;

    for (size_t idx = 0; idx != 10; idx++) {
        total_size += get_file_size(paths[idx]);

        if (strcmp(argv[1], "filtered") == 0) {
            process_filtered(paths[idx]);
        } else if (strcmp(argv[1], "unfiltered") == 0) {
            process_unfiltered(paths[idx]);
        } else {
            fprintf(stderr, "Unknown argument: >>>%s<<<\n", argv[1]);
            return 1;
        }
    }

    fprintf(stdout, "Size of all logs: %zu MiB\n", total_size / (1024 * 1024));
}
