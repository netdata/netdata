### Understand the alert

This alert indicates that the Pi-hole blocklist (Gravity) file hasn't been updated for an extended period of time. The blocklist file contains domains that have been processed by Pi-hole to filter ads and malicious content. An outdated blocklist may leave your system more vulnerable to unwanted content and threats.

### Troubleshoot the alert

1. **Check the current blocklist update status**

   To see how long it has been since the last update, you can use the following command:

   ```
   root@netdata~ # pihole -q -adlist
   ```

   This will display the timestamp of the last update.

2. **Rebuild the blocklist**

   If the alert indicates that your blocklist file is outdated, it's essential to update it by running:

   ```
   root@netdata~ # pihole -g
   ```

   This command will download the necessary files and rebuild the blocklist.

3. **Check for errors during the update**

   If you encounter any issues during the update, check the `/var/log/pihole.log` file for errors. You can also check the `/var/log/pihole-FTL.log` file for more detailed information on the update process.

4. **Verify the blocklist update interval**

   To ensure that your blocklist file is updated regularly, make sure you configure a regular update interval. You can do this by editing the `cron` job for Pi-hole:

   ```
   root@netdata~ # crontab -e
   ```

   This will open an editor. Look for the line containing the `pihole -g` command and adjust the schedule accordingly. For example, to update the blocklist daily, add the following line:

   ```
   0 0 * * * /usr/local/bin/pihole -g
   ```

   Save the file and exit the editor to apply the changes.

5. **Monitor the blocklist update status**

   After performing the necessary troubleshooting steps, keep an eye on the `pihole_blocklist_last_update` alert to ensure that your blocklist file is updated as expected.

### Useful resources

1. [Pi-hole Blocklists](https://docs.pi-hole.net/database/gravity/)
2. [Rebuilding the Blocklist](https://docs.pi-hole.net/ftldns/blockingmode/)
3. [Pi-hole Documentation](https://docs.pi-hole.net/)