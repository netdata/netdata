# megacli

Module collects adapter, physical drives and battery stats.

**Requirements:**
 * `netdata` user needs to be able to be able to sudo the `megacli` program without password

To grab stats it executes:
 * `sudo -n megacli -LDPDInfo -aAll`
 * `sudo -n megacli -AdpBbuCmd -a0`


It produces:

1. **Adapter State**

2. **Physical Drives Media Errors**

3. **Physical Drives Predictive Failures**

4. **Battery Relative State of Charge**

5. **Battery Cycle Count**

### configuration
Battery stats disabled by default in the module configuration file.

---
