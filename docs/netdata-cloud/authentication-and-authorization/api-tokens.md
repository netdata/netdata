# API Tokens

API tokens (Bearer tokens) enable you to access Netdata resources programmatically. These tokens authenticate and authorize API requests, allowing you to interact with Netdata services securely from external applications, scripts, or integrations.

:::important

API tokens never expire but should be managed carefully as they grant access to your Netdata resources.

:::

## Token Generation

**Location**

You can access token management through the Netdata UI:

1. Click your profile picture in the bottom-left corner
2. Select "User Settings"
3. Navigate to the API Tokens section

**Available Scopes**

You can limit each token to specific scopes that define its access permissions:

| Scope                  | Description                                                                                                                                                    | API Access                         |
| :--------------------- | :------------------------------------------------------------------------------------------------------------------------------------------------------------- | :--------------------------------- |
| `scope:all`            | Grants the same permissions as the user who created the token. Use case: Terraform provider integration.                                                       | Full access to all API endpoints   |
| `scope:agent-ui`       | Used by Agent for accessing the Cloud UI                                                                                                                       | Access to UI-related endpoints     |
| `scope:grafana-plugin` | Used for the [Netdata Grafana plugin](https://github.com/netdata/netdata-grafana-datasource-plugin/blob/master/README.md) to access Netdata charts             | Access to chart and data endpoints |
| `scope:mcp`            | Used to connect MCP clients (Claude Desktop, Cursor, etc.) to [Netdata Cloud MCP](/docs/netdata-ai/mcp/README.md#netdata-cloud-mcp) for AI-assisted monitoring | Access to MCP server endpoints     |

## API Versions

Netdata provides three API versions that you can access with API tokens:

- **v1**: The original API, focused on single-node operations
- **v2**: Multi-node API with advanced grouping and aggregation capabilities
- **v3**: The latest API version that combines v1 and v2 endpoints and may include additional features

## Common Endpoints

With appropriate API tokens, you can access endpoints including:

- `/api/v2/nodes` - Node information
- `/api/v2/data` - Multi-dimensional data queries
- `/api/v2/contexts` - Context metadata
- `/api/v2/weights` - Metric scoring/correlation
- `/api/v2/q` - Full-text search
- `/api/v1/info` - Agent information
- `/api/v1/charts` - Chart information
- `/api/v1/data` - Single node data queries

:::info

Currently, Netdata Cloud is not exposing the stable API.

:::

## Example Usage

**Get the Netdata Cloud space list**

```console
curl -H 'Accept: application/json' -H "Authorization: Bearer <token>" https://app.netdata.cloud/api/v2/spaces
```

**Get node information**

```console
curl -H 'Accept: application/json' -H "Authorization: Bearer <token>" https://app.netdata.cloud/api/v2/nodes
```

**Query metric data**

```console
curl -H 'Accept: application/json' -H "Authorization: Bearer <token>" https://app.netdata.cloud/api/v2/data?contexts=system.cpu&after=-600
```

**Get context information**

```console
curl -H 'Accept: application/json' -H "Authorization: Bearer <token>" https://app.netdata.cloud/api/v2/contexts
```

## Troubleshooting HTTP 412 Errors

If you receive an HTTP 412 (Precondition Failed) error when accessing Netdata Cloud API endpoints, this indicates that you are trying to access an authenticated endpoint without providing a valid Bearer token in the `Authorization` header.

In the context of Netdata Cloud, HTTP 412 specifically means that the API request is missing the required authorization token, rather than indicating a generic HTTP precondition failure.

**How to resolve HTTP 412 errors:**

1. **Generate an API token** from your Netdata Cloud settings:
   - Click your profile picture in the bottom-left corner
   - Select "User Settings"
   - Navigate to the API Tokens section
   - Create a new token with appropriate scopes for your use case

2. **Include the token in your request headers**:

   ```console
   curl -H 'Accept: application/json' -H "Authorization: Bearer <your-token-here>" https://app.netdata.cloud/api/v2/nodes
   ```

3. **Verify the token format**: Ensure you're using the full token string and that it starts with "Bearer " (note the space after "Bearer")

**Common causes:**

- Missing `Authorization` header entirely
- Using an invalid or revoked token
- Incorrect header format (missing "Bearer " prefix)
- Token with insufficient scope for the requested endpoint
