### Understand the alert

This alert monitors if Pi-hole's ability of blocking unwanted domains is active. If you receive this alert, it means that your Pi-hole's ad filtering is currently disabled.

### Troubleshoot the alert

1. Check the status of Pi-hole

To check the current status of Pi-hole, run the following command:
```
pihole status
```
This command will show if Pi-hole is active or disabled.

2. Re-enable Pi-hole

If Pi-hole is disabled as per the status, you can re-enable it by running the following command:

```
pihole enable
```

3. Confirm Pi-hole is enabled

After running the previous command, run `pihole status` again to confirm that Pi-hole is now enabled and blocking unwanted domains.

4. Check for errors or warnings

If Pi-hole is still not enabled, take a look at the logs for any errors or warnings:

```
cat /var/log/pihole.log | grep -i error
cat /var/log/pihole.log | grep -i warning
```

5. Rebuild the blocklist

If you still face issues, you can try rebuilding the blocklist by running:

```
pihole -g
```

6. Update Pi-hole

If the problem persists, consider updating Pi-hole to the latest version:

```
pihole -up
```

### Useful resources

1. [Pi-hole Official Documentation](https://docs.pi-hole.net/)
