# adaptec raid

Module collects logical and physical devices health metrics.

**Requirements:**
* `arcconf` program
* `sudo` program
* `netdata` user needs to be able to sudo the `arcconf` program without password

To grab stats it executes:
 * `sudo -n arcconf GETCONFIG 1 LD`
 * `sudo -n arcconf GETCONFIG 1 PD`


It produces:

1. **Logical Device Status**

2. **Physical Device State**

3. **Physical Device S.M.A.R.T warnings**

4. **Physical Device Temperature**

### prerequisite
This module uses `arcconf` which can only be executed by root.  It uses
`sudo` and assumes that it is configured such that the `netdata` user can
execute `arcconf` as root without password.

Add to `sudoers`:

    netdata ALL=(root)       NOPASSWD: /path/to/arcconf

### configuration

 **adaptec_raid** is disabled by default. Should be explicitly enabled in `python.d.conf`.

```yaml
adaptec_raid: yes
```

#### Screenshot:

![image](https://user-images.githubusercontent.com/22274335/47278133-6d306680-d601-11e8-87c2-cc9c0f42d686.png)

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fadaptec_raid%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
