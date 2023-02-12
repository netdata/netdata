<!--
title: "OpenLDAP monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/openldap/README.md"
sidebar_label: "OpenLDAP"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Networking"
-->

# OpenLDAP monitoring with Netdata

Provides statistics information from openldap (slapd) server.
Statistics are taken from LDAP monitoring interface. Manual page, slapd-monitor(5) is available.

**Requirement:**

-   Follow instructions from <https://www.openldap.org/doc/admin24/monitoringslapd.html> to activate monitoring interface.
-   Install python ldap module `pip install ldap` or `yum install python-ldap`
-   Modify openldap.conf with your credentials

### Module gives information with following charts:

1.  **connections**

    -   total connections number

2.  **Bytes**

    -   sent

3.  **operations**

    -   completed
    -   initiated

4.  **referrals**

    -   sent

5.  **entries**

    -   sent

6.  **ldap operations**

    -   bind
    -   search
    -   unbind 
    -   add
    -   delete
    -   modify
    -   compare

7.  **waiters**

    -   read
    -   write

## Configuration

Edit the `python.d/openldap.conf` configuration file using `edit-config` from the Netdata [config
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/openldap.conf
```

Sample:

```yaml
openldap:
  name     : 'local'
  username : "cn=monitor,dc=superb,dc=eu"
  password : "testpass"
  server   : 'localhost'
  port     : 389
```

---


