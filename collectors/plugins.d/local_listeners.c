#include <stdio.h>
#include <stdlib.h>
#include <stdbool.h>
#include <dirent.h>
#include <string.h>
#include <unistd.h>
#include <errno.h>
#include <ctype.h>
#include <arpa/inet.h>

typedef enum {
    PROC_NET_PROTOCOL_TCP,
    PROC_NET_PROTOCOL_TCP6,
    PROC_NET_PROTOCOL_UDP,
    PROC_NET_PROTOCOL_UDP6,
} PROC_NET_PROTOCOLS;

static inline const char *protocol_name(PROC_NET_PROTOCOLS protocol) {
    switch(protocol) {
        default:
        case PROC_NET_PROTOCOL_TCP:
            return "TCP";

        case PROC_NET_PROTOCOL_UDP:
            return "UDP";

        case PROC_NET_PROTOCOL_TCP6:
            return "TCP6";

        case PROC_NET_PROTOCOL_UDP6:
            return "UDP6";
    }
}

#define HASH_TABLE_SIZE 100000
#define MAX_ERROR_LOGS 10

typedef struct Node {
    unsigned int inode;
    unsigned int port;
    char local_address[INET6_ADDRSTRLEN];
    PROC_NET_PROTOCOLS protocol;
    bool processed;
    struct Node *next;
} Node;

typedef struct HashTable {
    Node *table[HASH_TABLE_SIZE];
} HashTable;

static size_t pid_fds_processed = 0;
static size_t pid_fds_failed = 0;
static size_t errors_encountered = 0;

static HashTable *hashTable_key_inode_port_value = NULL;

static inline void generate_output(const char *protocol, const char *address, unsigned int port, const char *cmdline) {
    printf("%s|%s|%u|%s\n", protocol, address, port, cmdline);
}

HashTable* createHashTable() {
    HashTable *hashTable = (HashTable*)malloc(sizeof(HashTable));
    memset(hashTable, 0, sizeof(HashTable));
    return hashTable;
}

static inline unsigned int hashFunction(unsigned int inode) {
    return inode % HASH_TABLE_SIZE;
}

static inline void insertHashTable(HashTable *hashTable, unsigned int inode, unsigned int port, PROC_NET_PROTOCOLS protocol, char *local_address) {
    unsigned int index = hashFunction(inode);
    Node *newNode = (Node*)malloc(sizeof(Node));
    newNode->inode = inode;
    newNode->port = port;
    newNode->protocol = protocol;
    strncpy(newNode->local_address, local_address, INET6_ADDRSTRLEN);
    newNode->local_address[INET6_ADDRSTRLEN - 1] = '\0';
    newNode->next = hashTable->table[index];
    hashTable->table[index] = newNode;
}

static inline unsigned int lookupHashTable(HashTable *hashTable, unsigned int inode, PROC_NET_PROTOCOLS *protocol, char **local_address) {
    unsigned int index = hashFunction(inode);
    Node *node = hashTable->table[index];
    while (node) {
        if (node->inode == inode) {
            *protocol = node->protocol;
            *local_address = node->local_address;
            node->processed = true;
            return node->port;
        }
        node = node->next;
    }
    return 0;  // Not found
}

void freeHashTable(HashTable *hashTable) {
    for (unsigned int i = 0; i < HASH_TABLE_SIZE; i++) {
        Node *node = hashTable->table[i];
        while (node) {
            Node *tmp = node;
            if(!tmp->processed)
                generate_output(protocol_name(tmp->protocol), tmp->local_address, tmp->port, "");
            node = node->next;
            free(tmp);
        }
    }
    free(hashTable);
}

static inline int read_cmdline(pid_t pid, char* buffer, size_t bufferSize) {
    char path[FILENAME_MAX + 1];
    snprintf(path, sizeof(path), "/proc/%d/cmdline", pid);
    path[FILENAME_MAX] = '\0';

    FILE* file = fopen(path, "r");
    if (!file) {
        if(++errors_encountered < MAX_ERROR_LOGS)
            fprintf(stderr, "local-listeners: error opening file: %s\n", path);

        return -1;
    }

    size_t bytesRead = fread(buffer, 1, bufferSize - 1, file);
    buffer[bytesRead] = '\0';  // Ensure null-terminated

    // Replace null characters in cmdline with spaces
    for (size_t i = 0; i < bytesRead; i++) {
        if (buffer[i] == '\0') {
            buffer[i] = ' ';
        }
    }

    fclose(file);
    return 0;
}

static inline void fix_cmdline(char* str) {
    if (str == NULL)
        return;

    char *s = str;

    do {
        if(*s == '|' || iscntrl(*s))
            *s = '_';

    } while(*++s);


    while(s > str && *(s-1) == ' ')
        *--s = '\0';
}

static inline void found_this_socket_inode(pid_t pid, unsigned int inode) {
    PROC_NET_PROTOCOLS protocol = 0;
    char *address = NULL;
    unsigned int port = lookupHashTable(hashTable_key_inode_port_value, inode, &protocol, &address);

    if(port) {
        char cmdline[8192] = "";
        read_cmdline(pid, cmdline, sizeof(cmdline));
        fix_cmdline(cmdline);
        generate_output(protocol_name(protocol), address, port, cmdline);
    }
}

bool find_all_sockets_in_proc(const char *proc_filename) {
    DIR *proc_dir, *fd_dir;
    struct dirent *proc_entry, *fd_entry;
    char path_buffer[FILENAME_MAX + 1];

    proc_dir = opendir(proc_filename);
    if (proc_dir == NULL) {
        if(++errors_encountered < MAX_ERROR_LOGS)
            fprintf(stderr, "local-listeners: cannot opendir() '%s' (error: %s)\n", proc_filename, strerror(errno));

        pid_fds_failed++;
        return false;
    }

    while ((proc_entry = readdir(proc_dir)) != NULL) {
        // Check if directory entry is a PID by seeing if the name is made up of digits only
        int is_pid = 1;
        for (char *c = proc_entry->d_name; *c != '\0'; c++) {
            if (*c < '0' || *c > '9') {
                is_pid = 0;
                break;
            }
        }

        if (!is_pid)
            continue;

        // Build the path to the fd directory of the process
        snprintf(path_buffer, FILENAME_MAX, "%s/%s/fd/", proc_filename, proc_entry->d_name);
        path_buffer[FILENAME_MAX] = '\0';

        fd_dir = opendir(path_buffer);
        if (fd_dir == NULL) {
            if(++errors_encountered < MAX_ERROR_LOGS)
                fprintf(stderr, "local-listeners: cannot opendir() '%s' (error: %s)\n", path_buffer, strerror(errno));

            pid_fds_failed++;
            continue;
        }

        while ((fd_entry = readdir(fd_dir)) != NULL) {
            if(!strcmp(fd_entry->d_name, ".") || !strcmp(fd_entry->d_name, ".."))
                continue;

            char link_path[512];
            char link_target[512];
            int inode;

            // Build the path to the file descriptor link
            strncpy(link_path, path_buffer, sizeof(link_path));
            strncat(link_path, fd_entry->d_name, sizeof(link_path) - strlen(link_path) - 1);

            ssize_t len = readlink(link_path, link_target, sizeof(link_target) - 1);
            if (len == -1) {
                if(++errors_encountered < MAX_ERROR_LOGS)
                    fprintf(stderr, "local-listeners: cannot read link '%s' (error: %s)\n", link_path, strerror(errno));

                pid_fds_failed++;
                continue;
            }
            link_target[len] = '\0';

            pid_fds_processed++;

            // If the link target indicates a socket, print its inode number
            if (sscanf(link_target, "socket:[%d]", &inode) == 1)
                found_this_socket_inode((pid_t)strtoul(proc_entry->d_name, NULL, 10), inode);
        }

        closedir(fd_dir);
    }

    closedir(proc_dir);
    return true;
}

static inline void add_port_and_inode(PROC_NET_PROTOCOLS protocol, unsigned int port, unsigned int inode, char *local_address) {
    insertHashTable(hashTable_key_inode_port_value, inode, port, protocol, local_address);
}

static inline void print_ipv6_address(const char *ipv6_str, char *dst) {
    unsigned k;
    char buf[9];
    struct sockaddr_in6 sa;

    // Initialize sockaddr_in6
    memset(&sa, 0, sizeof(struct sockaddr_in6));
    sa.sin6_family = AF_INET6;
    sa.sin6_port = htons(0); // replace 0 with your port number

    // Convert hex string to byte array
    for (k = 0; k < 4; ++k)
    {
        memset(buf, 0, 9);
        memcpy(buf, ipv6_str + (k * 8), 8);
        sa.sin6_addr.s6_addr32[k] = strtoul(buf, NULL, 16);
    }

    // Convert to human-readable format
    if (inet_ntop(AF_INET6, &(sa.sin6_addr), dst, INET6_ADDRSTRLEN) == NULL)
        *dst = '\0';
}

static inline void print_ipv4_address(uint32_t address, char *dst) {
    uint8_t octets[4];
    octets[0] = address & 0xFF;
    octets[1] = (address >> 8) & 0xFF;
    octets[2] = (address >> 16) & 0xFF;
    octets[3] = (address >> 24) & 0xFF;
    sprintf(dst, "%u.%u.%u.%u", octets[0], octets[1], octets[2], octets[3]);
}

bool read_proc_net_x(const char *filename, PROC_NET_PROTOCOLS protocol) {
    FILE *fp;
    char *line = NULL;
    size_t len = 0;
    ssize_t read;
    char address[INET6_ADDRSTRLEN];

    ssize_t min_line_length = (protocol == PROC_NET_PROTOCOL_TCP || protocol == PROC_NET_PROTOCOL_UDP) ? 105 : 155;

    fp = fopen(filename, "r");
    if (fp == NULL)
        return false;

    // Read line by line
    while ((read = getline(&line, &len, fp)) != -1) {
        if(read < min_line_length) continue;

        char local_address6[33], rem_address6[33];
        unsigned int local_address, local_port, state, rem_address, rem_port, inode;

        switch(protocol) {
            case PROC_NET_PROTOCOL_TCP:
                if(line[34] != '0' || line[35] != 'A')
                    continue;
                // fall-through

            case PROC_NET_PROTOCOL_UDP:
                if (sscanf(line, "%*d: %X:%X %X:%X %X %*X:%*X %*X:%*X %*X %*d %*d %u",
                    &local_address, &local_port, &rem_address, &rem_port, &state, &inode) != 6)
                    continue;

                print_ipv4_address(local_address, address);
                break;

            case PROC_NET_PROTOCOL_TCP6:
                if(line[82] != '0' || line[83] != 'A')
                    continue;
                    // fall-through

            case PROC_NET_PROTOCOL_UDP6:
                if(sscanf(line, "%*d: %32[0-9A-Fa-f]:%X %32[0-9A-Fa-f]:%X %X %*X:%*X %*X:%*X %*X %*d %*d %u",
                    local_address6, &local_port, rem_address6, &rem_port, &state, &inode) != 6)
                    continue;

                print_ipv6_address(local_address6, address);
                break;
        }

        add_port_and_inode(protocol, local_port, inode, address);
    }

    fclose(fp);
    if (line)
        free(line);

    return true;
}

int main(int argc, char **argv) {
    (void)argc;
    (void)argv;

    char *netdata_configured_host_prefix = getenv("NETDATA_HOST_PREFIX");
    if(!netdata_configured_host_prefix) netdata_configured_host_prefix = "";

    char path[FILENAME_MAX + 1];

    hashTable_key_inode_port_value = createHashTable();

    snprintf(path, FILENAME_MAX, "%s/proc/net/tcp", netdata_configured_host_prefix);
    path[FILENAME_MAX] = '\0';
    read_proc_net_x(path, PROC_NET_PROTOCOL_TCP);

    snprintf(path, FILENAME_MAX, "%s/proc/net/udp", netdata_configured_host_prefix);
    path[FILENAME_MAX] = '\0';
    read_proc_net_x(path, PROC_NET_PROTOCOL_UDP);

    snprintf(path, FILENAME_MAX, "%s/proc/net/tcp6", netdata_configured_host_prefix);
    path[FILENAME_MAX] = '\0';
    read_proc_net_x(path, PROC_NET_PROTOCOL_TCP6);

    snprintf(path, FILENAME_MAX, "%s/proc/net/udp6", netdata_configured_host_prefix);
    path[FILENAME_MAX] = '\0';
    read_proc_net_x(path, PROC_NET_PROTOCOL_UDP6);

    snprintf(path, FILENAME_MAX, "%s/proc", netdata_configured_host_prefix);
    path[FILENAME_MAX] = '\0';
    find_all_sockets_in_proc(path);

    freeHashTable(hashTable_key_inode_port_value);

    return 0;
}
