# API Tokens

API tokens (Bearer tokens) enable programmatic access to Netdata resources. These tokens authenticate and authorize API requests, allowing you to interact with Netdata services securely from external applications, scripts, or integrations.

> **Important**: API tokens never expire but should be managed carefully as they grant access to your Netdata resources.

## Token Generation

**Location**:

Access token management through the Netdata UI:

1. Click your profile picture in the bottom-left corner
2. Select "User Settings"
3. Navigate to the API Tokens section

**Available Scopes**:

Each token can be limited to specific scopes that define its access permissions:

| Scope                  | Description                                                                                                                                        |
|:-----------------------|:---------------------------------------------------------------------------------------------------------------------------------------------------|
| `scope:all`            | Grants the same permissions as the user who created the token. Use case: Terraform provider integration.                                           |
| `scope:agent-ui`       | Used by Agent for accessing the Cloud UI                                                                                                           |
| `scope:grafana-plugin` | Used for the [Netdata Grafana plugin](https://github.com/netdata/netdata-grafana-datasource-plugin/blob/master/README.md) to access Netdata charts |

> **Info**
>
> Currently, Netdata Cloud is not exposing the stable API.

## Example usage

**get the Netdata Cloud space list**

```console
curl -H 'Accept: application/json' -H "Authorization: Bearer <token>" https://app.netdata.cloud/api/v2/spaces
```
