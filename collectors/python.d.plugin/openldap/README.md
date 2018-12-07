# openldap

This module provides statistics information from openldap (slapd) server.
Statistics are taken from LDAP monitoring interface. Manual page, slapd-monitor(5) is available.

**Requirement:**
* Follow instructions from https://www.openldap.org/doc/admin24/monitoringslapd.html to activate monitoring interface.
* Install python ldap module `pip install ldap` or `yum install python-ldap`
* Modify openldap.conf with your credentials

### Module gives information with following charts:

1. **connections**
 * total connections number

2. **Bytes**
 * sent

3. **operations**
 * completed
 * initiated

4. **referrals**
 * sent

5. **entries**
 * sent

6. **ldap operations**
 * bind
 * search
 * unbind 
 * add
 * delete
 * modify
 * compare

7. **waiters**
 * read
 * write



### configuration

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

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fopenldap%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
