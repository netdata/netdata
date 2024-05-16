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

When integrating with WolfSSL, it's essential to confirm that the version of WolfSSL being used has enabled support for
the OpenSSL API during compilation. Failure to do so will result in compilation errors.
