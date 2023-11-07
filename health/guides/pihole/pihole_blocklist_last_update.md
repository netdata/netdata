# pihole_blocklist_last_update

## Ad Filtering | Pi-hole

This alert presents how much time has passed from the time the blocklist file (Gravity) was 
updated in seconds.  
Receiving this means that the blocklist file has not been updated for a long time.  

- This alert is raised to warning when the time in seconds exceeds 8 days.
- If the metric exceeds 16 days, then the alert is raised to critical.

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
To rebuild the blocklist, run the command:

```
root@netdata~ # pihole -g
```
</details>
