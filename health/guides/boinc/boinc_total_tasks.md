# boinc_total_tasks

**Computing | BOINC**

The Berkeley Open Infrastructure for Network Computing (BOINC) is an open-source middleware system
for volunteer computing and grid computing. The Netdata Agent monitors the average number of total
tasks over the last 10 minutes.

### Troubleshooting sections

<details>

<summary>Verify the project status</summary>

Verify that the projects you contribute are not suspended. Check if the project has queued tasks to
be done (https://boinc.berkeley.edu/projects.php)

</details>

<details>

<summary>Restart the BOINC client</summary>

1. In most of the linux distros

    ```
    root@netdata # /etc/init.d/boinc-client restart
    ```

</details>
