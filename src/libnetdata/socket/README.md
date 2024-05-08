<!--
Title: "Socket"
custom_edit_url: https://github.com/netdata/netdata/edit/master/src/libnetdata/socket/README.md
sidebar_label: "Socket"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Developers/libnetdata"
-->

# WolfSSL support

Support for WolfSSL is currently in the experimental stage, as it does not yet offer all the features available in the
OpenSSL library.

When linking against WolfSSL, it's crucial to ensure that the WolfSSL version has enabled support for the OpenSSL API.
You can verify this support by checking the following lines inside `/usr/include/wolfssl/options.h`:

```c
#undef  OPENSSL_NO_EC
#define OPENSSL_NO_EC

#undef  WOLFSSL_OPENSSH
#define WOLFSSL_OPENSSH
```
