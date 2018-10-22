# adaptec raid

Module collects logical and physical devices health metrics.

**Requirements:**
 * `netdata` user needs to be able to sudo the `arcconf` program without password

To grab stats it executes:
 * `sudo -n arcconf GETCONFIG 1 LD`
 * `sudo -n arcconf GETCONFIG 1 PD`


It produces:

1. **Logical Device Status**

2. **Physical Device State**

3. **Physical Device S.M.A.R.T warnings**

4. **Physical Device Temperature**

Screenshot:

![image](https://user-images.githubusercontent.com/22274335/47278133-6d306680-d601-11e8-87c2-cc9c0f42d686.png)

---
