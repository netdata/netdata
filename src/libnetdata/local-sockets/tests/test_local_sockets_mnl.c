// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/local-sockets/local-sockets.h"

static bool expect_result(int actual, int expected, int actual_errno, int expected_errno, const char *message) {
    if(actual == expected && actual_errno == expected_errno)
        return true;

    fprintf(
        stderr,
        "%s: expected result %d and errno %d, got result %d and errno %d\n",
        message,
        expected,
        expected_errno,
        actual,
        actual_errno);
    return false;
}

static struct nlmsghdr *put_control_message(void *buf, uint16_t type, const void *payload, size_t payload_len) {
    struct nlmsghdr *nlh = mnl_nlmsg_put_header(buf);
    nlh->nlmsg_type = type;

    if(payload_len) {
        void *dst = mnl_nlmsg_put_extra_header(nlh, payload_len);
        memcpy(dst, payload, payload_len);
    }

    return nlh;
}

static bool test_done_without_payload(void) {
    char buf[256] = { 0 };
    struct nlmsghdr *nlh = put_control_message(buf, NLMSG_DONE, NULL, 0);

    errno = 0;
    int result = local_sockets_libmnl_cb_run(buf, nlh->nlmsg_len, 0, 0, NULL, NULL);
    return expect_result(result, MNL_CB_STOP, errno, 0, "NLMSG_DONE without payload");
}

static bool test_done_success(void) {
    char buf[256] = { 0 };
    int done = 0;
    struct nlmsghdr *nlh = put_control_message(buf, NLMSG_DONE, &done, sizeof(done));

    errno = 0;
    int result = local_sockets_libmnl_cb_run(buf, nlh->nlmsg_len, 0, 0, NULL, NULL);
    return expect_result(result, MNL_CB_STOP, errno, 0, "successful NLMSG_DONE");
}

static bool test_done_error(void) {
    char buf[256] = { 0 };
    int done = -ENOENT;
    struct nlmsghdr *nlh = put_control_message(buf, NLMSG_DONE, &done, sizeof(done));

    errno = 0;
    int result = local_sockets_libmnl_cb_run(buf, nlh->nlmsg_len, 0, 0, NULL, NULL);
    return expect_result(result, MNL_CB_ERROR, errno, ENOENT, "error-bearing NLMSG_DONE");
}

static bool test_nlmsg_error(void) {
    char buf[256] = { 0 };
    struct nlmsgerr error = { .error = -EOPNOTSUPP };
    struct nlmsghdr *nlh = put_control_message(buf, NLMSG_ERROR, &error, sizeof(error));

    errno = 0;
    int result = local_sockets_libmnl_cb_run(buf, nlh->nlmsg_len, 0, 0, NULL, NULL);
    return expect_result(result, MNL_CB_ERROR, errno, EOPNOTSUPP, "NLMSG_ERROR");
}

static bool test_nlmsg_error_ack(void) {
    char buf[256] = { 0 };
    struct nlmsgerr error = { .error = 0 };
    struct nlmsghdr *nlh = put_control_message(buf, NLMSG_ERROR, &error, sizeof(error));

    errno = 0;
    int result = local_sockets_libmnl_cb_run(buf, nlh->nlmsg_len, 0, 0, NULL, NULL);
    return expect_result(result, MNL_CB_STOP, errno, 0, "successful NLMSG_ERROR acknowledgment");
}

static bool test_truncated_nlmsg_error(void) {
    char buf[256] = { 0 };
    struct nlmsghdr *nlh = put_control_message(buf, NLMSG_ERROR, NULL, 0);

    errno = 0;
    int result = local_sockets_libmnl_cb_run(buf, nlh->nlmsg_len, 0, 0, NULL, NULL);
    return expect_result(result, MNL_CB_ERROR, errno, EBADMSG, "truncated NLMSG_ERROR");
}

static int data_callback(const struct nlmsghdr *nlh __maybe_unused, void *data) {
    size_t *calls = data;
    (*calls)++;
    return MNL_CB_OK;
}

static bool test_data_message(void) {
    char buf[256] = { 0 };
    struct nlmsghdr *nlh = mnl_nlmsg_put_header(buf);
    nlh->nlmsg_type = SOCK_DIAG_BY_FAMILY;
    size_t calls = 0;

    errno = 0;
    int result = local_sockets_libmnl_cb_run(buf, nlh->nlmsg_len, 0, 0, data_callback, &calls);
    return expect_result(result, MNL_CB_OK, errno, 0, "socket data message") && calls == 1;
}

int main(void) {
    bool ok = true;

    ok = test_done_without_payload() && ok;
    ok = test_done_success() && ok;
    ok = test_done_error() && ok;
    ok = test_nlmsg_error() && ok;
    ok = test_nlmsg_error_ack() && ok;
    ok = test_truncated_nlmsg_error() && ok;
    ok = test_data_message() && ok;

    return ok ? 0 : 1;
}
