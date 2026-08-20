<!--
custom_edit_url: "https://github.com/netdata/netdata/edit/master/src/libnetdata/circular_buffer/README.md"
sidebar_label: "Circular Buffer"
learn_status: "Published"
learn_rel_path: "Developer and Contributor Corner/libnetdata"
learn_link: "https://learn.netdata.cloud/docs/developer-and-contributor-corner/libnetdata/circular-buffer"
slug: "/developer-and-contributor-corner/libnetdata/circular-buffer"
-->



# Circular Buffer

`struct circular_buffer` is an adaptive circular buffer. It will start at an initial size
and grow up to a maximum size as it fills. Two indices within the structure track the current
`read` and `write` position for data.
