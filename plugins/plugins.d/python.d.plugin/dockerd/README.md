# dockerd

Module monitor docker health metrics.

**Requirement:**
* `docker` package

Following charts are drawn:

1. **running containers**
 * count

2. **healthy containers**
 * count

3. **unhealthy containers**
 * count

### configuration

```yaml
 update_every : 1
 priority     : 60000
 ```

---
