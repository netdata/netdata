# pihole_status

## Ad Filtering | Pi-hole

This alert monitors if Pi-hole's ability of blocking unwanted domains is active.

- It is triggered in a warning state if pi-hole is disabled.


### Troubleshooting Section

<details>
<summary>Rebuild the blocklist</summary>
To fix, run the command:

```
root@netdata~ # pihole enable
```

*This feature should be enabled. The whole point of Pi-hole!*
</details>