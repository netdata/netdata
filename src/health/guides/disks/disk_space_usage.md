### Understand the alert

This alarm presents the percentage of used space of a particular disk. If it is close to 100%, it means that your storage device is running out of space. If the particular disk raising the alarm is full, the system could experience slowdowns and even crashes.

### Troubleshoot the alert

Clean or upgrade the drive.

If your storage device is full and the alert is raised, there are two paths you can tend to:

- Cleanup your drive, remove any unnecessary files (files on the trash directory, cache files etc.) to free up space. Some areas that are safe to delete, are:
  - Files under `/var/cache`
  - Old logs in `/var/log`
  - Old crash reports in `/var/crash` or `/var/dump`
  - The `.cache` directory in user home directories

- If your workflow requires all the space that is currently used, then you might want to look into upgrading the disk that raised the alarm, because its capacity is small for your demands.

Netdata strongly suggests that you are careful when cleaning up drives, and removing files, make sure that you are certain that you delete only unnecessary files.