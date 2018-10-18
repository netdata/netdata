# adaptec raid

Module collects logical and physical devices health metrics.

**Requirements:**
 * `netdata` user needs to be able to sudo the `arrconf` program without password

To grab stats it executes:
 * `sudo -n arrconf -GETCONFIG 1 LD`
 * `sudo -n arrconf -GETCONFIG 1 PD`


It produces:

1. **Logical Device Status**

2. **Physical Device State**

3. **Physical Device S.M.A.R.T warnings**

4. **Physical Device Temperature**

---
