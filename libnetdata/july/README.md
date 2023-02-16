<!--
custom_edit_url: https://github.com/netdata/netdata/edit/master/libnetdata/july/README.md
sidebar_label: "July interface"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Developers/libnetdata"
-->


# July

An interface similar to `Judy` that uses minimal allocations (that can be cached)
for items that are mainly appended (just a few insertions in the middle)

