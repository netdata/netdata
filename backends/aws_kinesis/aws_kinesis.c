// SPDX-License-Identifier: GPL-3.0-or-later

#define BACKENDS_INTERNALS
#include "aws_kinesis.h"

#define CONFIG_FILE_LINE_MAX ((CONFIG_MAX_NAME + CONFIG_MAX_VALUE + 1024) * 2)

// ----------------------------------------------------------------------------
// kinesis backend

// read the aws_kinesis.conf file
int read_kinesis_conf(const char *path, char **access_key_id_p, char **secret_access_key_p, char **stream_name_p)
{
    char *access_key_id = *access_key_id_p;
    char *secret_access_key = *secret_access_key_p;
    char *stream_name = *stream_name_p;

    if(unlikely(access_key_id)) freez(access_key_id);
    if(unlikely(secret_access_key)) freez(secret_access_key);
    if(unlikely(stream_name)) freez(stream_name);
    access_key_id = NULL;
    secret_access_key = NULL;
    stream_name = NULL;

    int line = 0;

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/aws_kinesis.conf", path);

    char buffer[CONFIG_FILE_LINE_MAX + 1], *s;

    debug(D_BACKEND, "BACKEND: opening config file '%s'", filename);

    FILE *fp = fopen(filename, "r");
    if(!fp) {
        return 1;
    }

    while(fgets(buffer, CONFIG_FILE_LINE_MAX, fp) != NULL) {
        buffer[CONFIG_FILE_LINE_MAX] = '\0';
        line++;

        s = trim(buffer);
        if(!s || *s == '#') {
            debug(D_BACKEND, "BACKEND: ignoring line %d of file '%s', it is empty.", line, filename);
            continue;
        }

        char *name = s;
        char *value = strchr(s, '=');
        if(unlikely(!value)) {
            error("BACKEND: ignoring line %d ('%s') of file '%s', there is no = in it.", line, s, filename);
            continue;
        }
        *value = '\0';
        value++;

        name = trim(name);
        value = trim(value);

        if(unlikely(!name || *name == '#')) {
            error("BACKEND: ignoring line %d of file '%s', name is empty.", line, filename);
            continue;
        }

        if(!value)
            value = "";
        else
            value = strip_quotes(value);

        if(name[0] == 'a' && name[4] == 'a' && !strcmp(name, "aws_access_key_id")) {
            access_key_id = strdupz(value);
        }
        else if(name[0] == 'a' && name[4] == 's' && !strcmp(name, "aws_secret_access_key")) {
            secret_access_key = strdupz(value);
        }
        else if(name[0] == 's' && !strcmp(name, "stream name")) {
            stream_name = strdupz(value);
        }
    }

    fclose(fp);

    if(unlikely(!stream_name || !*stream_name)) {
        error("BACKEND: stream name is a mandatory Kinesis parameter but it is not configured");
        return 1;
    }

    *access_key_id_p = access_key_id;
    *secret_access_key_p = secret_access_key;
    *stream_name_p = stream_name;

    return 0;
}
