#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <stdbool.h>

#define MAX_SEARCH 3
#define MAX_PARAMETERS 128
#define ERROR_BUFFER_SIZE 1024

struct command {
    const char *name;
    const char *params;
    const char *search[MAX_SEARCH];
} allowed_commands[] = {
    {
        .name = "ethtool-module-info",
        .params = "-m {{devname}}",
        .search =
            {
                [0] = "ethtool",
                [1] = NULL,
            },
    },
    {
        .name = "chronyc-serverstats",
        .params = "serverstats",
        .search =
            {
                [0] = "chronyc",
                [1] = NULL,
            },
    },
    {
        .name = "varnishadm-backend-list",
        .params = "backend.list",
        .search =
            {
                [0] = "varnishadm",
                [1] = NULL,
            },
    },
    {
        .name = "varnishstat-stats",
        .params = "-1 -t off -n {{instanceName}}",
        .search =
            {
                [0] = "varnishstat",
                [1] = NULL,
            },
    },
    {
        .name = "smbstatus-profile",
        .params = "-P",
        .search =
            {
                [0] = "smbstatus",
                [1] = NULL,
            },
    },
    {
        .name = "exim-bpc",
        .params = "-bpc",
        .search =
            {
                [0] = "exim",
                [1] = NULL,
            },
    },
    {
        .name = "nsd-control-stats",
        .params = "stats_noreset",
        .search = {
            [0] = "nsd-control",
            [1] = NULL,
        },
    },
    {
        .name = "chronyc-serverstats",
        .params = "serverstats",
        .search = {
            [0] = "chronyc",
            [1] = NULL,
        },
    },
    {
        .name = "dmsetup-status-cache",
        .params = "status --target cache --noflush",
        .search = {
            [0] = "dmsetup",
            [1] = NULL,
        },
    },
    {
        .name = "ssacli-controllers-info",
        .params = "ctrl all show config detail",
        .search = {
            [0] = "ssacli",
            [1] = NULL,
        },
    },
    {
        .name = "smartctl-json-scan",
        .params = "--json --scan",
        .search = {
            [0] = "smartctl",
            [1] = NULL,
        },
    },
    {
        .name = "smartctl-json-scan-open",
        .params = "--json --scan-open",
        .search = {
            [0] = "smartctl",
            [1] = NULL,
        },
    },
    {
        .name = "smartctl-json-device-info",
        .params = "--json --all {{deviceName}} --device {{deviceType}} --nocheck {{powerMode}}",
        .search = {
            [0] = "smartctl",
            [1] = NULL,
        },
    },
    {
        .name = "fail2ban-client-status",
        .params = "status",
        .search = {
            [0] = "fail2ban-client",
            [1] = NULL,
        },
    },
    {
        .name = "fail2ban-client-status-socket",
        .params = "-s {{socket_path}} status",
        .search = {
            [0] = "fail2ban-client",
            [1] = NULL,
        },
    },
    {
        .name = "fail2ban-client-status-jail",
        .params = "status {{jail}}",
        .search = {
            [0] = "fail2ban-client",
            [1] = NULL,
        },
    },
    {
        .name = "fail2ban-client-status-jail-socket",
        .params = "-s {{socket_path}} status {{jail}}",
        .search = {
            [0] = "fail2ban-client",
            [1] = NULL,
        },
    },
    {
        .name = "storcli-controllers-info",
        .params = "/cALL show all J nolog",
        .search = {
            [0] = "storcli",
            [1] = NULL,
        },
    },
    {
        .name = "storcli-drives-info",
        .params = "/cALL/eALL/sALL show all J nolog",
        .search = {
            [0] = "storcli",
            [1] = NULL,
        },
    },
    {
        .name = "lvs-report-json",
        .params = "--reportformat json --units b --nosuffix -o {{options}}",
        .search = {
            [0] = "lvs",
            [1] = NULL,
        },
    },
    {
        .name = "igt-list-gpus",
        .params = "-L",
        .search = {
            [0] = "intel_gpu_top",
            [1] = NULL,
        },
    },
    {
        .name = "igt-device-json",
        .params = "-d {{device}} -J -s {{interval}}",
        .search = {
            [0] = "intel_gpu_top",
            [1] = NULL,
        },
    },
    {
        .name = "igt-json",
        .params = "-J -s {{interval}}",
        .search = {
            [0] = "intel_gpu_top",
            [1] = NULL,
        },
    },
    {
        .name = "nvme-list",
        .params = "list --output-format=json",
        .search = {
            [0] = "nvme",
            [1] = NULL,
        },
    },
    {
        .name = "nvme-smart-log",
        .params = "smart-log {{device}} --output-format=json",
        .search = {
            [0] = "nvme",
            [1] = NULL,
        },
    },
    {
        .name = "megacli-disk-info",
        .params = "-LDPDInfo -aAll -NoLog",
        .search = {
            [0] = "megacli",
            [1] = "MegaCli",
            [2] = "MegaCli64",
        },
    },
    {
        .name = "megacli-battery-info",
        .params = "-AdpBbuCmd -aAll -NoLog",
        .search = {
            [0] = "megacli",
            [1] = "MegaCli",
            [2] = "MegaCli64",
        },
    },
    {
        .name = "arcconf-ld-info",
        .params = "GETCONFIG 1 LD",
        .search = {
            [0] = "arcconf",
            [1] = NULL,
        },
    },
    {
        .name = "arcconf-pd-info",
        .params = "GETCONFIG 1 PD",
        .search = {
            [0] = "arcconf",
            [1] = NULL,
        },
    }
};

bool command_exists_in_dir(const char *dir, const char *cmd, char *dst, size_t dst_size) {
    snprintf(dst, dst_size, "%s/%s", dir, cmd);
    return access(dst, X_OK) == 0;
}

bool command_exists_in_PATH(const char *cmd, char *dst, size_t dst_size) {
    if(!dst || !dst_size)
        return false;

    char *path = getenv("PATH");
    if(!path)
        return false;

    char *path_copy = strdup(path);
    if (!path_copy)
        return false;

    char *dir;
    bool found = false;
    dir = strtok(path_copy, ":");
    while(dir && !found) {
        found = command_exists_in_dir(dir, cmd, dst, dst_size);
        dir = strtok(NULL, ":");
    }

    free(path_copy);
    return found;
}

struct command *find_command(const char *cmd) {
    size_t size = sizeof(allowed_commands) / sizeof(allowed_commands[0]);
    for(size_t i = 0; i < size ;i++) {
        if(strcmp(cmd, allowed_commands[i].name) == 0)
            return &allowed_commands[i];
    }

    return NULL;
}

bool check_string(const char *str, size_t index, char *err, size_t err_size) {
    const char *s = str;
    while(*s) {
        char c = *s++;
        if(!((c >= 'A' && c <= 'Z') ||
             (c >= 'a' && c <= 'z') ||
             (c >= '0' && c <= '9') ||
              c == ' ' || c == '_' || c == '-' || c == '/' || 
              c == '.' || c == ',' || c == ':' || c == '=')) {
            snprintf(err, err_size, "command line argument No %zu includes invalid character '%c'", index, c);
            return false;
        }
    }

    return true;
}

bool check_params(int argc, char **argv, char *err, size_t err_size) {
    for(int i = 0 ; i < argc ;i++)
        if(!check_string(argv[i], i, err, err_size))
            return false;

    return true;
}

char *find_variable_in_argv(const char *variable, int argc, char **argv, char *err, size_t err_size) {
    for (int i = 1; i < argc - 1; i++) {
        if (strcmp(argv[i], variable) == 0)
            return strdup(argv[i + 1]);
    }

    snprintf(err, err_size, "variable '%s' is required, but was not provided in the command line parameters", variable);

    return NULL;
}

bool search_and_replace_params(struct command *cmd, char **params, size_t max_params, const char *filename, int argc, char **argv, char *err, size_t err_size) {
    if (!cmd || !params || !max_params) {
        snprintf(err, err_size, "search_and_replace_params() internal error");
        return false;
    }

    const char *delim = " ";
    char *token;
    char *temp_params = strdup(cmd->params);
    if (!temp_params) {
        snprintf(err, err_size, "search_and_replace_params() cannot allocate memory");
        return false;
    }

    size_t param_count = 0;
    params[param_count++] = strdup(filename);

    token = strtok(temp_params, delim);
    while (token && param_count < max_params - 1) {
        size_t len = strlen(token);

        char *value = NULL;

        if (strncmp(token, "{{", 2) == 0 && strncmp(token + len - 2, "}}", 2) == 0) {
            token[0] = '-';
            token[1] = '-';
            token[len - 2] = '\0';

            value = find_variable_in_argv(token, argc, argv, err, err_size);
        }
        else
            value = strdup(token);

        if(!value)
            goto cleanup;

        params[param_count++] = value;
        token = strtok(NULL, delim);
    }

    params[param_count] = NULL; // Null-terminate the params array
    free(temp_params);
    return true;

cleanup:
    if(!err[0])
        snprintf(err, err_size, "memory allocation failure");

    free(temp_params);
    for (size_t i = 0; i < param_count; ++i) {
        free(params[i]);
        params[i] = NULL;
    }
    return false;
}

void show_help() {
    fprintf(stdout, "\n");
    fprintf(stdout, "ndsudo\n");
    fprintf(stdout, "\n");
    fprintf(stdout, "Copyright 2018-2025 Netdata Inc.\n");
    fprintf(stdout, "\n");
    fprintf(stdout, "A helper to allow Netdata run privileged commands.\n");
    fprintf(stdout, "\n");
    fprintf(stdout, "  --test\n");
    fprintf(stdout, "    print the generated command that will be run, without running it.\n");
    fprintf(stdout, "\n");
    fprintf(stdout, "  --help\n");
    fprintf(stdout, "    print this message.\n");
    fprintf(stdout, "\n");

    fprintf(stdout, "The following commands are supported:\n\n");

    size_t size = sizeof(allowed_commands) / sizeof(allowed_commands[0]);
    for(size_t i = 0; i < size ;i++) {
        fprintf(stdout, "- Command    : %s\n", allowed_commands[i].name);
        fprintf(stdout, "  Executables: ");
        for(size_t j = 0; j < MAX_SEARCH && allowed_commands[i].search[j] ;j++) {
            fprintf(stdout, "%s ", allowed_commands[i].search[j]);
        }
        fprintf(stdout, "\n");
        fprintf(stdout, "  Parameters : %s\n\n", allowed_commands[i].params);
    }

    fprintf(stdout, "The program searches for executables in the system path.\n");
    fprintf(stdout, "\n");
    fprintf(stdout, "Variables given as {{variable}} are expected on the command line as:\n");
    fprintf(stdout, "  --variable VALUE\n");
    fprintf(stdout, "\n");
    fprintf(stdout, "VALUE can include space, A-Z, a-z, 0-9, _, -, /, and .\n");
    fprintf(stdout, "\n");
}

int main(int argc, char *argv[]) {
    char error_buffer[ERROR_BUFFER_SIZE] = "";

    if (argc < 2) {
        fprintf(stderr, "at least 2 parameters are needed, but %d were given.\n", argc);
        return 1;
    }

    if(!check_params(argc, argv, error_buffer, sizeof(error_buffer))) {
        fprintf(stderr, "invalid characters in parameters: %s\n", error_buffer);
        return 2;
    }

    bool test = false;
    const char *cmd = argv[1];
    if(strcmp(cmd, "--help") == 0 || strcmp(cmd, "-h") == 0) {
        show_help();
        exit(0);
    }
    else if(strcmp(cmd, "--test") == 0) {
        cmd = argv[2];
        test = true;
    }

    struct command *command = find_command(cmd);
    if(!command) {
        fprintf(stderr, "command not recognized: %s\n", cmd);
        return 3;
    }

    char new_path[] = "PATH=/bin:/sbin:/usr/bin:/usr/sbin:/usr/local/bin:/usr/local/sbin";
    putenv(new_path);

    setuid(0);
    setgid(0);
    setegid(0);

    bool found = false;
    char filename[FILENAME_MAX];

    for(size_t i = 0; i < MAX_SEARCH && !found ;i++) {
        if(command->search[i]) {
            found = command_exists_in_PATH(command->search[i], filename, sizeof(filename));
            if(!found) {
                size_t len = strlen(error_buffer);
                snprintf(&error_buffer[len], sizeof(error_buffer) - len, "%s ", command->search[i]);
            }
        }
    }

    if(!found) {
        fprintf(stderr, "%s: not available in PATH.\n", error_buffer);
        return 4;
    }
    else
        error_buffer[0] = '\0';

    char *params[MAX_PARAMETERS];
    if(!search_and_replace_params(command, params, MAX_PARAMETERS, filename, argc, argv, error_buffer, sizeof(error_buffer))) {
        fprintf(stderr, "command line parameters are not satisfied: %s\n", error_buffer);
        return 5;
    }

    if(test) {
        fprintf(stderr, "Command to run: \n");

        for(size_t i = 0; i < MAX_PARAMETERS && params[i] ;i++)
            fprintf(stderr, "'%s' ", params[i]);

        fprintf(stderr, "\n");

        exit(0);
    }
    else {
        char *clean_env[] = {NULL};
        execve(filename, params, clean_env);
        perror("execve"); // execve only returns on error
        return 6;
    }
}
