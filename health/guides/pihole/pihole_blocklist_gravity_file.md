# pihole_blocklist_gravity_file

## Ad Filtering | Pi-hole

This alert indicates the existence of the blocklist file. If you receive this, it means that the
gravity.list file (blocklist) is non-existent.

- The alert is raised in a critical state when the metric gets the value of 1.

> The gravity table consists of the domains that have been processed by Pi-hole's gravity
> (pihole -g) command. The domains in this list are the collection of domains sourced from the
> configured sources (see the adlist table).<sup>[1](
> https://docs.pi-hole.net/database/gravity/#gravity-table-gravity) </sup>
<details><summary>References and Sources</summary>

1. [Pi-hole Docs](https://docs.pi-hole.net/database/gravity/#gravity-table-gravity)

</details>

### Troubleshooting Section

<details>
<summary>Rebuild the blocklist</summary>
To rebuild the gravity.list (blocklist), run the command:

```
root@netdata~ # pihole -g
```

</details>