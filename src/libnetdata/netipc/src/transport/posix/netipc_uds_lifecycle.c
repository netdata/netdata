#include "netipc_uds_internal.h"

#include <dirent.h>
#include <errno.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/un.h>

static int validate_service_name(const char *name)
{
    if (!name || name[0] == '\0')
        return -1;

    if (name[0] == '.' && (name[1] == '\0' ||
                           (name[1] == '.' && name[2] == '\0')))
        return -1;

    for (const char *p = name; *p; p++) {
        char c = *p;
        if ((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
            (c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-')
            continue;
        return -1;
    }
    return 0;
}

int nipc_uds_build_socket_name(char *dst, size_t dst_len,
                               const char *service_name)
{
    if (validate_service_name(service_name) < 0)
        return -2;

    int n = snprintf(dst, dst_len, "%s.sock", service_name);
    if (n < 0 || (size_t)n >= dst_len)
        return -1;
    return 0;
}

int nipc_uds_build_socket_path(char *dst, size_t dst_len,
                               const char *run_dir,
                               const char *service_name)
{
    char socket_name[sizeof(((struct sockaddr_un *)0)->sun_path)];
    int name_rc = nipc_uds_build_socket_name(socket_name, sizeof(socket_name),
                                             service_name);
    if (name_rc < 0)
        return name_rc;

    int n = snprintf(dst, dst_len, "%s/%s", run_dir, socket_name);
    if (n < 0 || (size_t)n >= dst_len)
        return -1;
    return 0;
}

bool nipc_uds_run_dir_allows_stale_unlink(const char *run_dir)
{
    struct stat st;
    if (stat(run_dir, &st) != 0)
        return false;
    if (!S_ISDIR(st.st_mode))
        return false;
    if (st.st_uid != geteuid())
        return false;
    return (st.st_mode & (S_IWGRP | S_IWOTH)) == 0;
}

static int unlink_stale_socket_path(const char *run_dir,
                                    const char *socket_name,
                                    bool allow_stale_unlink)
{
    if (!allow_stale_unlink)
        return -1;

    DIR *dir = opendir(run_dir);
    if (!dir)
        return -1;

    int dir_fd = dirfd(dir);
    if (dir_fd < 0) {
        closedir(dir);
        return -1;
    }

    struct stat st;
    if (fstatat(dir_fd, socket_name, &st, AT_SYMLINK_NOFOLLOW) != 0) {
        int ret = (errno == ENOENT) ? 0 : -1;
        closedir(dir);
        return ret;
    }

    if (!S_ISSOCK(st.st_mode)) {
        closedir(dir);
        return -1;
    }

    int ret = -1;
    if (unlinkat(dir_fd, socket_name, 0) == 0 || errno == ENOENT)
        ret = 0;

    closedir(dir);
    return ret;
}

int nipc_uds_check_and_recover_stale(const char *run_dir,
                                     const char *socket_name,
                                     const char *path,
                                     bool allow_stale_unlink)
{
    int probe = socket(AF_UNIX, SOCK_SEQPACKET, 0);
    if (probe < 0)
        return -1;

    struct sockaddr_un addr;
    memset(&addr, 0, sizeof(addr));
    addr.sun_family = AF_UNIX;
    strncpy(addr.sun_path, path, sizeof(addr.sun_path) - 1);

    int ret;
    if (connect(probe, (struct sockaddr *)&addr, sizeof(addr)) == 0) {
        close(probe);
        ret = 1;
    } else {
        int saved_errno = errno;
        close(probe);
        if (saved_errno == ENOENT) {
            ret = -1;
        } else if (saved_errno == ECONNREFUSED) {
            ret = (unlink_stale_socket_path(run_dir, socket_name,
                                            allow_stale_unlink) == 0) ? 0 : 1;
        } else {
            ret = 1;
        }
    }
    return ret;
}

nipc_uds_error_t nipc_uds_listen(const char *run_dir,
                                  const char *service_name,
                                  const nipc_uds_server_config_t *config,
                                  nipc_uds_listener_t *out)
{
    memset(out, 0, sizeof(*out));
    out->fd = -1;

    char path[sizeof(((struct sockaddr_un *)0)->sun_path)];
    int path_rc = nipc_uds_build_socket_path(path, sizeof(path), run_dir,
                                             service_name);
    if (path_rc == -2)
        return NIPC_UDS_ERR_BAD_PARAM;
    if (path_rc < 0)
        return NIPC_UDS_ERR_PATH_TOO_LONG;

    char socket_name[sizeof(((struct sockaddr_un *)0)->sun_path)];
    int name_rc = nipc_uds_build_socket_name(socket_name, sizeof(socket_name),
                                             service_name);
    if (name_rc == -2)
        return NIPC_UDS_ERR_BAD_PARAM;
    if (name_rc < 0)
        return NIPC_UDS_ERR_PATH_TOO_LONG;

    bool allow_stale_unlink = nipc_uds_run_dir_allows_stale_unlink(run_dir);
    int stale = nipc_uds_check_and_recover_stale(run_dir, socket_name, path,
                                                allow_stale_unlink);
    if (stale == 1)
        return NIPC_UDS_ERR_ADDR_IN_USE;

    int fd = socket(AF_UNIX, SOCK_SEQPACKET, 0);
    if (fd < 0)
        return NIPC_UDS_ERR_SOCKET;

    struct sockaddr_un addr;
    memset(&addr, 0, sizeof(addr));
    addr.sun_family = AF_UNIX;
    strncpy(addr.sun_path, path, sizeof(addr.sun_path) - 1);

    if (bind(fd, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
        close(fd);
        return NIPC_UDS_ERR_SOCKET;
    }

    int backlog = config->backlog > 0 ? config->backlog : UDS_DEFAULT_BACKLOG;
    if (listen(fd, backlog) < 0) {
        close(fd);
        unlink(path);
        return NIPC_UDS_ERR_SOCKET;
    }

    out->fd = fd;
    out->config = *config;
    strncpy(out->path, path, sizeof(out->path) - 1);
    out->path[sizeof(out->path) - 1] = '\0';

    return NIPC_UDS_OK;
}

nipc_uds_error_t nipc_uds_accept(nipc_uds_listener_t *listener,
                                  uint64_t session_id,
                                  nipc_uds_session_t *out)
{
    memset(out, 0, sizeof(*out));
    out->fd = -1;

    int client_fd = accept(listener->fd, NULL, NULL);
    if (client_fd < 0)
        return NIPC_UDS_ERR_ACCEPT;

    nipc_uds_error_t err = nipc_uds_server_handshake(
        client_fd, &listener->config, session_id, out);
    if (err != NIPC_UDS_OK) {
        close(client_fd);
        out->fd = -1;
        return err;
    }

    return NIPC_UDS_OK;
}

nipc_uds_error_t nipc_uds_connect(const char *run_dir,
                                   const char *service_name,
                                   const nipc_uds_client_config_t *config,
                                   nipc_uds_session_t *out)
{
    memset(out, 0, sizeof(*out));
    out->fd = -1;

    char path[sizeof(((struct sockaddr_un *)0)->sun_path)];
    int path_rc = nipc_uds_build_socket_path(path, sizeof(path), run_dir,
                                             service_name);
    if (path_rc == -2)
        return NIPC_UDS_ERR_BAD_PARAM;
    if (path_rc < 0)
        return NIPC_UDS_ERR_PATH_TOO_LONG;

    int fd = socket(AF_UNIX, SOCK_SEQPACKET, 0);
    if (fd < 0)
        return NIPC_UDS_ERR_SOCKET;

    struct sockaddr_un addr;
    memset(&addr, 0, sizeof(addr));
    addr.sun_family = AF_UNIX;
    strncpy(addr.sun_path, path, sizeof(addr.sun_path) - 1);

    if (connect(fd, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
        close(fd);
        return NIPC_UDS_ERR_CONNECT;
    }

    nipc_uds_error_t err = nipc_uds_client_handshake(fd, config, out);
    if (err != NIPC_UDS_OK) {
        close(fd);
        out->fd = -1;
        return err;
    }

    return NIPC_UDS_OK;
}

void nipc_uds_close_session(nipc_uds_session_t *session)
{
    if (!session)
        return;

    if (session->fd >= 0) {
        close(session->fd);
        session->fd = -1;
    }

    free(session->recv_buf);
    session->recv_buf = NULL;
    session->recv_buf_size = 0;

    free(session->inflight_ids);
    session->inflight_ids = NULL;
    session->inflight_count = 0;
    session->inflight_capacity = 0;
}

void nipc_uds_close_listener(nipc_uds_listener_t *listener)
{
    if (!listener)
        return;

    if (listener->fd >= 0) {
        close(listener->fd);
        listener->fd = -1;
    }

    if (listener->path[0]) {
        unlink(listener->path);
        listener->path[0] = '\0';
    }
}
