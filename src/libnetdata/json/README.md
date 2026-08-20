<!--
custom_edit_url: "https://github.com/netdata/netdata/edit/master/src/libnetdata/json/README.md"
sidebar_label: "JSON"
learn_status: "Published"
learn_rel_path: "Developer and Contributor Corner/libnetdata"
learn_link: "https://learn.netdata.cloud/docs/developer-and-contributor-corner/libnetdata/json"
slug: "/developer-and-contributor-corner/libnetdata/json"
-->



# json

`json` contains a parser for json strings, based on `jsmn` (<https://github.com/zserge/jsmn>), but case you have installed the JSON-C library, the installation script will prefer it, you can also force its use with `--enable-jsonc` in the compilation time.


