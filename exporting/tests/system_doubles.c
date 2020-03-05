// SPDX-License-Identifier: GPL-3.0-or-later

#include "test_exporting_engine.h"

void __wrap_uv_thread_create(uv_thread_t thread, void (*worker)(void *arg), void *arg)
{
    function_called();

    check_expected_ptr(thread);
    check_expected_ptr(worker);
    check_expected_ptr(arg);
}

void __wrap_uv_mutex_lock(uv_mutex_t *mutex)
{
    (void)mutex;
}

void __wrap_uv_mutex_unlock(uv_mutex_t *mutex)
{
    (void)mutex;
}

void __wrap_uv_cond_signal(uv_cond_t *cond_var)
{
    (void)cond_var;
}

void __wrap_uv_cond_wait(uv_cond_t *cond_var, uv_mutex_t *mutex)
{
    (void)cond_var;
    (void)mutex;
}

ssize_t __wrap_recv(int sockfd, void *buf, size_t len, int flags)
{
    function_called();

    check_expected(sockfd);
    check_expected_ptr(buf);
    check_expected(len);
    check_expected(flags);

    char *mock_string = "Test recv";
    strcpy(buf, mock_string);

    return strlen(mock_string);
}

ssize_t __wrap_send(int sockfd, const void *buf, size_t len, int flags)
{
    function_called();

    check_expected(sockfd);
    check_expected_ptr(buf);
    check_expected_ptr(buf);
    check_expected(len);
    check_expected(flags);

    return strlen(buf);
}
