// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"
#include <openssl/x509v3.h>

// A TLS client must check that the server certificate was issued for the host it connects to,
// not only that a trusted CA signed it. This runs a local TLS server with certificates signed by
// a throwaway CA the client trusts, and checks which ones nd_sock_connect_to_this() accepts.

#define ND_SOCK_TEST(condition, msg) do {                                        \
        if(!(condition)) {                                                       \
            fprintf(stderr, "nd_sock unittest FAILED: %s (%s:%d)\n",             \
                    (msg), __FUNCTION__, __LINE__);                              \
            errors++;                                                            \
        }                                                                        \
    } while(0)

static EVP_PKEY *nd_sock_test_key(void) {
    EVP_PKEY *pkey = NULL;
    EVP_PKEY_CTX *ctx = EVP_PKEY_CTX_new_id(EVP_PKEY_EC, NULL);

    if(!ctx || EVP_PKEY_keygen_init(ctx) <= 0 ||
        EVP_PKEY_CTX_set_ec_paramgen_curve_nid(ctx, NID_X9_62_prime256v1) <= 0 ||
        EVP_PKEY_keygen(ctx, &pkey) <= 0) {
        EVP_PKEY_free(pkey);
        pkey = NULL;
    }

    EVP_PKEY_CTX_free(ctx);
    return pkey;
}

static bool nd_sock_test_add_ext(X509 *cert, X509 *issuer, int nid, const char *value) {
    X509V3_CTX v3;
    X509V3_set_ctx_nodb(&v3);
    X509V3_set_ctx(&v3, issuer, cert, NULL, NULL, 0);

    X509_EXTENSION *ext = X509V3_EXT_conf_nid(NULL, &v3, nid, value);
    if(!ext)
        return false;

    bool ok = X509_add_ext(cert, ext, -1) == 1;
    X509_EXTENSION_free(ext);
    return ok;
}

// a CA certificate when issuer is NULL (self-signed), otherwise a server certificate for san
static X509 *nd_sock_test_cert(EVP_PKEY *key, const char *cn, X509 *issuer, EVP_PKEY *issuer_key, const char *san) {
    static long serial = 0;

    X509 *cert = X509_new();
    if(!cert)
        return NULL;

    X509 *signer = issuer ? issuer : cert;
    bool ok =
        X509_set_version(cert, 2) == 1 &&
        ASN1_INTEGER_set(X509_get_serialNumber(cert), ++serial) == 1 &&
        X509_gmtime_adj(X509_getm_notBefore(cert), -3600) &&
        X509_gmtime_adj(X509_getm_notAfter(cert), 86400) &&
        X509_set_pubkey(cert, key) == 1 &&
        X509_NAME_add_entry_by_txt(X509_get_subject_name(cert), "CN", MBSTRING_ASC,
                                   (const unsigned char *)cn, -1, -1, 0) == 1 &&
        X509_set_issuer_name(cert, X509_get_subject_name(signer)) == 1 &&
        nd_sock_test_add_ext(cert, signer, NID_basic_constraints, issuer ? "critical,CA:FALSE" : "critical,CA:TRUE") &&
        (issuer || nd_sock_test_add_ext(cert, signer, NID_key_usage, "critical,keyCertSign,cRLSign")) &&
        (!san || nd_sock_test_add_ext(cert, signer, NID_subject_alt_name, san)) &&
        X509_sign(cert, issuer ? issuer_key : key, EVP_sha256()) > 0;

    if(!ok) {
        X509_free(cert);
        return NULL;
    }

    return cert;
}

static SSL_CTX *nd_sock_test_server_ctx(X509 *cert, EVP_PKEY *key) {
    SSL_CTX *ctx = SSL_CTX_new(TLS_server_method());
    if(ctx && (SSL_CTX_use_certificate(ctx, cert) != 1 || SSL_CTX_use_PrivateKey(ctx, key) != 1)) {
        SSL_CTX_free(ctx);
        ctx = NULL;
    }
    return ctx;
}

struct nd_sock_test_server {
    int listen_fd;
    SSL_CTX *ctx;
};

static void nd_sock_test_server_thread(void *ptr) {
    struct nd_sock_test_server *srv = ptr;

    struct pollfd pfd = { .fd = srv->listen_fd, .events = POLLIN };
    if(poll(&pfd, 1, 10 * MSEC_PER_SEC) != 1)
        return;

    int fd = accept(srv->listen_fd, NULL, NULL);
    if(fd < 0)
        return;

    struct timeval tv = { .tv_sec = 10, .tv_usec = 0 };
    (void)setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof(tv));

    SSL *ssl = SSL_new(srv->ctx);
    if(ssl && SSL_set_fd(ssl, fd) == 1 && SSL_accept(ssl) == 1) {
        // keep the connection open until the client closes it, so neither side writes to a closed socket
        char c;
        int r;
        while((r = SSL_read(ssl, &c, 1)) > 0)
            ;

        if(SSL_get_error(ssl, r) == SSL_ERROR_ZERO_RETURN)
            SSL_shutdown(ssl);
    }

    SSL_free(ssl);
    close(fd);
}

// serve one TLS connection with server_ctx and connect to it as "host:port"
static bool nd_sock_test_connect(SSL_CTX *server_ctx, SSL_CTX *client_ctx, bool verify, const char *host, ND_SOCK_ERROR *error) {
    *error = ND_SOCK_ERR_UNKNOWN_ERROR;

    struct sockaddr_in sa = {
        .sin_family = AF_INET,
        .sin_port = 0,
        .sin_addr.s_addr = htonl(INADDR_LOOPBACK),
    };
    socklen_t sa_len = sizeof(sa);

    int listen_fd = socket(AF_INET, SOCK_STREAM, 0);
    if(listen_fd < 0)
        return false;

    if(bind(listen_fd, (struct sockaddr *)&sa, sizeof(sa)) != 0 ||
        listen(listen_fd, 1) != 0 ||
        getsockname(listen_fd, (struct sockaddr *)&sa, &sa_len) != 0) {
        close(listen_fd);
        return false;
    }

    struct nd_sock_test_server srv = {
        .listen_fd = listen_fd,
        .ctx = server_ctx,
    };

    ND_THREAD *thread = nd_thread_create("NDSOCK-TLS",
                                         NETDATA_THREAD_OPTION_DONT_LOG | NETDATA_THREAD_OPTION_MANUAL_JOIN,
                                         nd_sock_test_server_thread, &srv);
    if(!thread) {
        close(listen_fd);
        return false;
    }

    char destination[256];
    snprintfz(destination, sizeof(destination), "%s:%d", host, (int)ntohs(sa.sin_port));

    ND_SOCK sock = ND_SOCK_INIT(client_ctx, verify);
    bool connected = nd_sock_connect_to_this(&sock, destination, 0, 5, true);
    *error = sock.error;
    nd_sock_close(&sock);

    nd_thread_join(thread);
    close(listen_fd);

    return connected;
}

int nd_sock_ssl_unittest(void) {
    int errors = 0;
    fprintf(stderr, "\nTesting TLS peer verification of nd_sock_connect_to_this()...\n");

    EVP_PKEY *ca_key = nd_sock_test_key();
    EVP_PKEY *key = nd_sock_test_key();
    EVP_PKEY *untrusted_key = nd_sock_test_key();

    X509 *ca = ca_key ? nd_sock_test_cert(ca_key, "netdata unittest CA", NULL, NULL, NULL) : NULL;
    X509 *untrusted_ca = untrusted_key ? nd_sock_test_cert(untrusted_key, "localhost", NULL, NULL, "DNS:localhost,IP:127.0.0.1") : NULL;
    X509 *local = (ca && key) ? nd_sock_test_cert(key, "localhost", ca, ca_key, "DNS:localhost,IP:127.0.0.1") : NULL;
    X509 *other = (ca && key) ? nd_sock_test_cert(key, "parent.example", ca, ca_key, "DNS:parent.example") : NULL;

    SSL_CTX *local_ctx = local ? nd_sock_test_server_ctx(local, key) : NULL;
    SSL_CTX *other_ctx = other ? nd_sock_test_server_ctx(other, key) : NULL;
    SSL_CTX *untrusted_ctx = untrusted_ca ? nd_sock_test_server_ctx(untrusted_ca, untrusted_key) : NULL;

    // the same kind of context the streaming sender uses, trusting only the unittest CA
    SSL_CTX *client_ctx = netdata_ssl_create_client_ctx(SSL_MODE_ENABLE_PARTIAL_WRITE | SSL_MODE_ACCEPT_MOVING_WRITE_BUFFER);
    bool client_ok = client_ctx && ca && X509_STORE_add_cert(SSL_CTX_get_cert_store(client_ctx), ca) == 1;

    ND_SOCK_TEST(local_ctx && other_ctx && untrusted_ctx && client_ok, "cannot prepare the TLS test certificates");

    if(local_ctx && other_ctx && untrusted_ctx && client_ok) {
        ND_SOCK_ERROR error;

        ND_SOCK_TEST(nd_sock_test_connect(local_ctx, client_ctx, true, "127.0.0.1", &error),
                     "a certificate with the IP address connected to must be accepted");

        ND_SOCK_TEST(nd_sock_test_connect(local_ctx, client_ctx, true, "localhost", &error),
                     "a certificate with the hostname connected to must be accepted");

        ND_SOCK_TEST(!nd_sock_test_connect(other_ctx, client_ctx, true, "127.0.0.1", &error) &&
                         error == ND_SOCK_ERR_SSL_INVALID_CERTIFICATE,
                     "a trusted certificate issued for another name must be rejected when connecting to an IP");

        ND_SOCK_TEST(!nd_sock_test_connect(other_ctx, client_ctx, true, "localhost", &error) &&
                         error == ND_SOCK_ERR_SSL_INVALID_CERTIFICATE,
                     "a trusted certificate issued for another name must be rejected when connecting to a hostname");

        ND_SOCK_TEST(!nd_sock_test_connect(untrusted_ctx, client_ctx, true, "localhost", &error) &&
                         error == ND_SOCK_ERR_SSL_INVALID_CERTIFICATE,
                     "a certificate from an untrusted issuer must be rejected");

        ND_SOCK_TEST(nd_sock_test_connect(other_ctx, client_ctx, false, "127.0.0.1", &error),
                     "skipping certificate verification must accept any certificate");
    }

    SSL_CTX_free(client_ctx);
    SSL_CTX_free(untrusted_ctx);
    SSL_CTX_free(other_ctx);
    SSL_CTX_free(local_ctx);
    X509_free(other);
    X509_free(local);
    X509_free(untrusted_ca);
    X509_free(ca);
    EVP_PKEY_free(untrusted_key);
    EVP_PKEY_free(key);
    EVP_PKEY_free(ca_key);
    ERR_clear_error();

    fprintf(stderr, "nd_sock TLS peer verification unittest: %s (%d errors)\n", errors ? "FAILED" : "OK", errors);
    return errors;
}
