The file format is YAML. A single-instance collector takes exactly one job, whose name is fixed:

```yaml
jobs:
  - name: [[ entry.meta.module_name ]]
```

Any other job name is rejected. Set the collector's own options alongside `name`.
