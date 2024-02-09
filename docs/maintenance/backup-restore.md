<!--
title: "Backing up Netdata"
description: "Guide on how to backup and restore Netdata."
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/maintenance/backup-restore.md"
sidebar_label: "Notify"
learn_status: "Published"
learn_rel_path: "Integrations/Notify"
-->

# Introduction
When preparing to backup Netdata it is worth considering that there are different sorts of data that you may wish to backup independently or all together:

| Data type      | Description | Location |
| ----------- | ----------- | ----------- |
| Agent configuration| Files controlling configuration of the Netdata agent | /etc/netdata/
| Metrics   | Database files | /var/cache/netdata |
| Identity   | Cloud-claim, API key and some other files | /var/lib/netdata |

<br>
Note: Only you can decide how to perform your backups. The following examples are provided as a guide only.


<br>

# Backing up Netdata

## Scenarios

### 1. Backing up to restore data in case of node failure
In this standard scenario you are backing up your Netdata agent in case of some sort of node failure or data corruption so that the metrics and configuration-state can be recovered. The purpose is not to backup/restore the application itself.

`Step 1` Verify that the directory-paths in the table above contain the information that you expect.<br>
_The specific paths may vary depending upon installation method, Operating System and whether it is a Docker/Kubernetes deployment._

`Step 2` It is recommended that you stop the Netdata agent when backing up the Metrics/database files.
<br>
Backing up the Agent configuration and Identity folders is straight-forward as they should not be changing very frequently.

`Step 3`
Using a backup tool such as **tar** you will likely need to run the backup as _root_ or the _netdata_ user in order to access all the files for backup.
e.g.
```
sudo tar -cvpzf netdata_backup.tar.gz /etc/netdata/ /var/cache/netdata /var/lib/netdata
```
Stopping the Netdata agent is mostly required for backing up of the _database files_ and so if you wish to keep the gap in data from the stopping of the Netdata service to an absolute minimum then you could have a backup job or script that uses the following sequence:
- Backup the Agent configuration Identity directories
- Stop the Netdata service
- Backup up up the database files
- Restart the netdata agent.

<br>


# Restoring Netdata

`Step 1` Ensure that the Netdata agent is installed and in a **stopped state**. <br>
_If you plan to deploy the agent and restore the backup over the top of it then you might find it helpful to use the "--dont-start-it" switch, e.g._
```
wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh && sh /tmp/netdata-kickstart.sh --dont-start-it
```
_Regardless you must verify that the agent/service is actually stopped_

`Step 1.5` If you are going to restore the database files then you should _first_ **ensure that the Metrics directory is empty**:
```
sudo rm -Rf /var/cache/netdata
```

`Step 2` Restore from the backup archive
```
sudo tar -xvpzf /path/to/netdata_backup.tar.gz -C /
```

3. Start the Netdata agent