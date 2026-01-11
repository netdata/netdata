# 9.4 Health Management API

## 9.4.1 Enable/Disable Alerts

```bash
# Disable an alert
curl -s "http://localhost:19999/api/v1/health?cmd=disable&alarm=high_cpu"

# Enable an alert
curl -s "http://localhost:19999/api/v1/health?cmd=enable&alarm=high_cpu"
```

## 9.4.2 Reload Configuration

```bash
curl -s "http://localhost:19999/api/v1/health?cmd=reload"
```

Equivalent to `netdatacli reload-health`.

## 9.4.3 Related Sections

- **4.1 Disabling Alerts** for configuration-based disabling
- **8.4 Custom Actions** for exec-based automation