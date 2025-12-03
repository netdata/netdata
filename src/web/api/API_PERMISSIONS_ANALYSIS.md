# Netdata API Permissions and Access Control Analysis

**Generated:** 2025-10-02
**Purpose:** Document ACL and HTTP_ACCESS requirements for all 68 Netdata APIs

## Permission System Overview

### HTTP_ACL (Access Control List)
Controls **HOW** the API can be accessed (transport/source):

- `HTTP_ACL_NOCHECK` - No ACL checking (always allow based on other criteria)
- `HTTP_ACL_API` - Via HTTP/HTTPS web server (TCP port 19999)
- `HTTP_ACL_ACLK` - **Via ACLK only** (Netdata Cloud connection)
- `HTTP_ACL_WEBRTC` - Via WebRTC connection
- `HTTP_ACL_METRICS` - Metrics data access category
- `HTTP_ACL_FUNCTIONS` - Functions execution category
- `HTTP_ACL_NODES` - Node information category
- `HTTP_ACL_ALERTS` - Alerts access category
- `HTTP_ACL_DYNCFG` - Dynamic configuration category
- `HTTP_ACL_REGISTRY` - Registry access category
- `HTTP_ACL_BADGES` - Badges generation category
- `HTTP_ACL_MANAGEMENT` - Management operations category
- `HTTP_ACL_STREAMING` - Streaming category
- `HTTP_ACL_NETDATACONF` - Netdata configuration category

### HTTP_ACCESS (Permission Flags)
Controls **WHAT** the authenticated user can do (capabilities):

- `HTTP_ACCESS_NONE` - No specific access required (public)
- `HTTP_ACCESS_SIGNED_ID` - User must be authenticated
- `HTTP_ACCESS_SAME_SPACE` - User and agent must be in same Netdata Cloud space
- `HTTP_ACCESS_COMMERCIAL_SPACE` - Requires commercial plan
- `HTTP_ACCESS_ANONYMOUS_DATA` - Can view basic metrics/data
- `HTTP_ACCESS_SENSITIVE_DATA` - Can view sensitive information
- `HTTP_ACCESS_VIEW_AGENT_CONFIG` - Can read agent configuration
- `HTTP_ACCESS_EDIT_AGENT_CONFIG` - Can modify agent configuration
- `HTTP_ACCESS_VIEW_NOTIFICATIONS_CONFIG` - Can read notifications config
- `HTTP_ACCESS_EDIT_NOTIFICATIONS_CONFIG` - Can modify notifications config
- `HTTP_ACCESS_VIEW_ALERTS_SILENCING` - Can read silencing rules
- `HTTP_ACCESS_EDIT_ALERTS_SILENCING` - Can modify silencing rules

### HTTP_USER_ROLE
User roles with hierarchical permissions (lower number = more permissions):

1. `HTTP_USER_ROLE_ADMIN` - Full administrative access
2. `HTTP_USER_ROLE_MANAGER` - Management access
3. `HTTP_USER_ROLE_TROUBLESHOOTER` - Diagnostic access
4. `HTTP_USER_ROLE_OBSERVER` - Read-only access
5. `HTTP_USER_ROLE_MEMBER` - Basic member access
6. `HTTP_USER_ROLE_BILLING` - Billing-related access
7. `HTTP_USER_ROLE_ANY` - Any authenticated user

---

## V3 APIs (27 total) - CURRENT/LATEST

### Public Data APIs (Accessible by anyone, local or cloud)
These require only `HTTP_ACCESS_ANONYMOUS_DATA` - available to all users including unauthenticated:

| API | ACL | Access | Description |
|-----|-----|--------|-------------|
| `/api/v3/data` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` | Time-series data query |
| `/api/v3/badge.svg` | `HTTP_ACL_BADGES` | `ANONYMOUS_DATA` | Badge generation |
| `/api/v3/weights` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` | Scoring engine |
| `/api/v3/allmetrics` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` | Metrics export |
| `/api/v3/context` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` | Context metadata |
| `/api/v3/contexts` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` | Multi-node contexts |
| `/api/v3/q` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` | Full-text search |
| `/api/v3/alerts` | `HTTP_ACL_ALERTS` | `ANONYMOUS_DATA` | Multi-node alerts |
| `/api/v3/alert_transitions` | `HTTP_ACL_ALERTS` | `ANONYMOUS_DATA` | Alert history |
| `/api/v3/alert_config` | `HTTP_ACL_ALERTS` | `ANONYMOUS_DATA` | Alert configuration |
| `/api/v3/variable` | `HTTP_ACL_ALERTS` | `ANONYMOUS_DATA` | Chart variables |
| `/api/v3/nodes` | `HTTP_ACL_NODES` | `ANONYMOUS_DATA` | Nodes listing |
| `/api/v3/node_instances` | `HTTP_ACL_NODES` | `ANONYMOUS_DATA` | Node instances |
| `/api/v3/stream_path` | `HTTP_ACL_NODES` | `ANONYMOUS_DATA` | Streaming topology |
| `/api/v3/function` | `HTTP_ACL_FUNCTIONS` | `ANONYMOUS_DATA` | Execute function (permissions checked per-function) |
| `/api/v3/functions` | `HTTP_ACL_FUNCTIONS` | `ANONYMOUS_DATA` | List functions |
| `/api/v3/config` | `HTTP_ACL_DYNCFG` | `ANONYMOUS_DATA` | Dynamic configuration (read/write permissions checked per-action) |

### Public Info APIs (No authentication required)
These have `HTTP_ACL_NOCHECK` and `HTTP_ACCESS_NONE`:

| API | ACL | Access | Description |
|-----|-----|--------|-------------|
| `/api/v3/info` | `HTTP_ACL_NOCHECK` | `NONE` | Agent information |
| `/api/v3/versions` | `HTTP_ACL_NOCHECK` | `ANONYMOUS_DATA` | Version information |
| `/api/v3/progress` | `HTTP_ACL_NOCHECK` | `ANONYMOUS_DATA` | Function progress tracking |
| `/api/v3/settings` | `HTTP_ACL_NOCHECK` | `ANONYMOUS_DATA` | User settings (GET/PUT) |
| `/api/v3/stream_info` | `HTTP_ACL_NOCHECK` | `NONE` | Streaming statistics |
| `/api/v3/claim` | `HTTP_ACL_NOCHECK` | `NONE` | Agent claiming (security key required) |
| `/api/v3/me` | `HTTP_ACL_NOCHECK` | `NONE` | Current user info |

### ACLK-Only APIs (Netdata Cloud Access Required)
These require `HTTP_ACL_ACLK` - **ONLY accessible via Netdata Cloud (ACLK)**:

| API | ACL | Access | Requirements | Description |
|-----|-----|--------|--------------|-------------|
| `/api/v3/rtc_offer` | `HTTP_ACL_ACLK` | `SIGNED_ID` + `SAME_SPACE` | Authenticated user in same space | WebRTC connection establishment |
| `/api/v3/bearer_protection` | `HTTP_ACL_ACLK` | `SIGNED_ID` + `SAME_SPACE` + `VIEW_AGENT_CONFIG` + `EDIT_AGENT_CONFIG` | Admin/Manager role | Enable/disable bearer protection |
| `/api/v3/bearer_get_token` | `HTTP_ACL_ACLK` | `SIGNED_ID` + `SAME_SPACE` | Authenticated user in same space | Generate bearer token |

**Note:** `ACL_DEV_OPEN_ACCESS` flag makes these available in dev mode without ACLK restriction.

---

## V2 APIs (17 total) - DEPRECATED

All V2 APIs have same ACL/access as their V3 equivalents:

### Public Data APIs
| API | ACL | Access |
|-----|-----|--------|
| `/api/v2/data` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` |
| `/api/v2/weights` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` |
| `/api/v2/contexts` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` |
| `/api/v2/q` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` |
| `/api/v2/alerts` | `HTTP_ACL_ALERTS` | `ANONYMOUS_DATA` |
| `/api/v2/alert_transitions` | `HTTP_ACL_ALERTS` | `ANONYMOUS_DATA` |
| `/api/v2/alert_config` | `HTTP_ACL_ALERTS` | `ANONYMOUS_DATA` |
| `/api/v2/nodes` | `HTTP_ACL_NODES` | `ANONYMOUS_DATA` |
| `/api/v2/node_instances` | `HTTP_ACL_NODES` | `ANONYMOUS_DATA` |
| `/api/v2/versions` | `HTTP_ACL_NODES` | `ANONYMOUS_DATA` |
| `/api/v2/functions` | `HTTP_ACL_FUNCTIONS` | `ANONYMOUS_DATA` |

### Public Info APIs
| API | ACL | Access |
|-----|-----|--------|
| `/api/v2/info` | `HTTP_ACL_NOCHECK` | `ANONYMOUS_DATA` |
| `/api/v2/progress` | `HTTP_ACL_NOCHECK` | `ANONYMOUS_DATA` |
| `/api/v2/claim` | `HTTP_ACL_NOCHECK` | `NONE` |

### ACLK-Only APIs
| API | ACL | Access |
|-----|-----|--------|
| `/api/v2/rtc_offer` | `HTTP_ACL_ACLK` | `SIGNED_ID` + `SAME_SPACE` |
| `/api/v2/bearer_protection` | `HTTP_ACL_ACLK` | `SIGNED_ID` + `SAME_SPACE` + `VIEW_AGENT_CONFIG` + `EDIT_AGENT_CONFIG` |
| `/api/v2/bearer_get_token` | `HTTP_ACL_ACLK` | `SIGNED_ID` + `SAME_SPACE` |

---

## V1 APIs (24 total) - DEPRECATED

### Public Data APIs
| API | ACL | Access |
|-----|-----|--------|
| `/api/v1/data` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` |
| `/api/v1/weights` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` |
| `/api/v1/metric_correlations` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` |
| `/api/v1/badge.svg` | `HTTP_ACL_BADGES` | `ANONYMOUS_DATA` |
| `/api/v1/allmetrics` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` |
| `/api/v1/chart` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` |
| `/api/v1/charts` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` |
| `/api/v1/context` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` |
| `/api/v1/contexts` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` |
| `/api/v1/function` | `HTTP_ACL_FUNCTIONS` | `ANONYMOUS_DATA` |
| `/api/v1/functions` | `HTTP_ACL_FUNCTIONS` | `ANONYMOUS_DATA` |
| `/api/v1/config` | `HTTP_ACL_DYNCFG` | `ANONYMOUS_DATA` |

### Alert APIs
| API | ACL | Access |
|-----|-----|--------|
| `/api/v1/alarms` | `HTTP_ACL_ALERTS` | `ANONYMOUS_DATA` |
| `/api/v1/alarms_values` | `HTTP_ACL_ALERTS` | `ANONYMOUS_DATA` |
| `/api/v1/alarm_log` | `HTTP_ACL_ALERTS` | `ANONYMOUS_DATA` |
| `/api/v1/alarm_variables` | `HTTP_ACL_ALERTS` | `ANONYMOUS_DATA` |
| `/api/v1/variable` | `HTTP_ACL_ALERTS` | `ANONYMOUS_DATA` |
| `/api/v1/alarm_count` | `HTTP_ACL_ALERTS` | `ANONYMOUS_DATA` |

### Node Info APIs
| API | ACL | Access |
|-----|-----|--------|
| `/api/v1/info` | `HTTP_ACL_NODES` | `ANONYMOUS_DATA` |
| `/api/v1/aclk` | `HTTP_ACL_NODES` | `ANONYMOUS_DATA` |
| `/api/v1/dbengine_stats` | `HTTP_ACL_NODES` | `ANONYMOUS_DATA` |
| `/api/v1/ml_info` | `HTTP_ACL_NODES` | `ANONYMOUS_DATA` |

### Special APIs
| API | ACL | Access | Notes |
|-----|-----|--------|-------|
| `/api/v1/registry` | `HTTP_ACL_NONE` | `NONE` | Manages ACL internally |
| `/api/v1/manage` | `HTTP_ACL_MANAGEMENT` | `NONE` | Manages access internally, requires `HTTP_ACL_MANAGEMENT` |

---

## Security Summary by Category

### 1. ACLK-Only APIs (Cloud Access Required)
**Count:** 6 APIs (3 v3, 3 v2)

These APIs are ONLY accessible through Netdata Cloud (ACLK connection):
- `rtc_offer` - WebRTC setup (**Experimental feature, not compiled by default**)
- `bearer_protection` - Requires admin/manager permissions
- `bearer_get_token` - Generate authentication tokens

**Access Requirement:** User must be authenticated (`SIGNED_ID`) and in the same Netdata Cloud space as the agent (`SAME_SPACE`)

**Cannot be accessed via:** Direct HTTP/HTTPS to agent (even with bearer token)

**Note:** WebRTC (`rtc_offer`) is an experimental feature and is not compiled by default. It requires special build configuration.

### 2. Optionally Protected Data APIs (Bearer Protection Configurable)
**Count:** 47 APIs across all versions

These provide read access to metrics, alerts, and metadata with `HTTP_ACCESS_ANONYMOUS_DATA`.

**Default Mode (Public):**
- No authentication required
- Available to local dashboard, cloud users, external tools
- Subject to IP-based ACL restrictions in netdata.conf

**Bearer Protection Mode (when enabled):**
- Requires valid bearer token for access
- Bearer tokens obtained via `/api/v*/bearer_get_token` (ACLK-only)
- Still subject to IP-based ACL restrictions
- Provides token-based authentication layer

**Configuration:** Set bearer protection via `/api/v*/bearer_protection` API or netdata.conf

### 3. Configuration APIs (Permissions Checked Per-Action)
**Count:** 3 APIs

- `/api/v*/config` - Read operations allowed for all, write operations check permissions internally
- `/api/v*/function` - Execution permissions checked per-function by plugins
- `/api/v1/manage` - Manages permissions internally

**Note:** These respect bearer protection if enabled (they have `HTTP_ACCESS_ANONYMOUS_DATA`)

### 4. Always Public APIs (Cannot Be Restricted)
**Count:** 12 APIs

These have `HTTP_ACL_NOCHECK` meaning they bypass ALL security:
- Agent info
- Version info
- Progress tracking
- Current user info
- Settings (user preferences)
- Claiming (protected by security key mechanism, not ACL/bearer)

**Important:** These are ALWAYS accessible:
- NOT affected by bearer protection
- NOT subject to IP-based ACL
- Cannot be restricted by any configuration

**Important:** These APIs are ALWAYS public and cannot be restricted:
- NOT subject to IP-based ACL restrictions
- NOT subject to bearer protection
- Always accessible without authentication

---

## Permission Checking Flow

1. **ACL Check** (web_api.c:65-67)
   ```c
   bool acl_allows = ((w->acl & api_commands[i].acl) == api_commands[i].acl)
                     || (api_commands[i].acl & HTTP_ACL_NOCHECK);
   ```
   - Verifies request came through allowed transport (API, ACLK, WebRTC)
   - Verifies request matches allowed feature category

2. **Access Check** (web_api.c:69-72)
   ```c
   bool permissions_allows =
       http_access_user_has_enough_access_level_for_endpoint(
           w->user_auth.access, api_commands[i].access);
   ```
   - Verifies user has required permission flags
   - Checks user role has sufficient access level

3. **Internal Permission Checks**
   - Some APIs (config, function, registry, manage) perform additional permission validation within their implementation
   - These check specific actions or function-level permissions

---

## Configuration Impact

### netdata.conf ACL Settings
IP-based ACL restrictions can be applied to feature categories:

```conf
[web]
    allow connections from = localhost *
    allow dashboard from = *
    allow badges from = *
    allow streaming from = *
    allow netdata.conf from = localhost fd* 10.* 192.168.* 172.16.* 172.17.* 172.18.* 172.19.* 172.20.* 172.21.* 172.22.* 172.23.* 172.24.* 172.25.* 172.26.* 172.27.* 172.28.* 172.29.* 172.30.* 172.31.* UNKNOWN
    allow management from = localhost
```

These settings affect which IP addresses can access APIs in each category.

### Bearer Protection
When enabled via `bearer_protection` API or netdata.conf, APIs with `HTTP_ACCESS_ANONYMOUS_DATA` require bearer token authentication.

**Effect:**
- Changes security model from public/IP-based to token-based
- Only affects APIs with `HTTP_ACCESS_ANONYMOUS_DATA` (47 APIs)
- Does NOT affect `HTTP_ACL_NOCHECK` APIs (always public)
- Does NOT affect ACLK-only APIs (already require cloud authentication)

**Token Management:**
- Tokens obtained via `/api/v*/bearer_get_token` (requires ACLK access)
- Tokens are time-limited with expiration
- Tokens include role-based access control (admin, manager, etc.)

**Use Cases:**
- Securing public-facing Netdata agents
- Controlling access to metrics/alerts APIs
- Integrating with external authentication systems

---

## Recommendations for Swagger Documentation

Each API should document:

### 1. **Access Type (Primary Classification):**

**For ACLK-Only APIs:**
```
⚠️ **ACLK-Only API - Cloud Access Required**

This API is ONLY accessible via Netdata Cloud (ACLK). Direct HTTP/HTTPS access to the agent is not allowed.

**Requirements:**
- User must be authenticated via Netdata Cloud
- User and agent must be in the same Netdata Cloud space
- [Additional role requirements if applicable]
```

**For HTTP_ACCESS_ANONYMOUS_DATA APIs:**
```
📊 **Public Data API (Bearer Protection Configurable)**

**Default Mode:** Publicly accessible without authentication
**Bearer Protection Mode:** Requires valid bearer token when enabled

**Access Methods:**
- Direct HTTP/HTTPS to agent (default: public, configurable)
- Netdata Cloud (authenticated)
- External tools/integrations

**IP Restrictions:** Subject to [ACL category] restrictions in netdata.conf
**Bearer Protection:** Can be enabled via netdata.conf or `/api/v3/bearer_protection`
```

**For HTTP_ACL_NOCHECK APIs:**
```
🔓 **Always Public API**

This API is always publicly accessible and cannot be restricted.

**No Security:**
- Not subject to bearer protection
- Not subject to IP-based ACL restrictions
- No authentication required or possible
```

### 2. **ACL Category:**
Document which ACL category applies (for IP restriction):
- `HTTP_ACL_METRICS` → "allow dashboard from" in netdata.conf
- `HTTP_ACL_ALERTS` → "allow dashboard from" in netdata.conf
- `HTTP_ACL_NODES` → "allow dashboard from" in netdata.conf
- `HTTP_ACL_FUNCTIONS` → "allow dashboard from" in netdata.conf
- `HTTP_ACL_DYNCFG` → "allow dashboard from" in netdata.conf
- `HTTP_ACL_BADGES` → "allow badges from" in netdata.conf
- `HTTP_ACL_MANAGEMENT` → "allow management from" in netdata.conf

### 3. **Permission Details:**
- List required HTTP_ACCESS flags when applicable
- Note minimum user role for ACLK-only APIs
- Explain any per-action or per-function permission checks

### 4. **Bearer Protection Impact:**
```
**When Bearer Protection is Enabled:**
- Requires valid bearer token in Authorization header
- Token format: `Authorization: Bearer <token>`
- Tokens obtained via `/api/v3/bearer_get_token` (ACLK-only)
- Token expiration and renewal required
```

---

**Last Updated:** 2025-10-02
**Next Action:** Add security documentation to all 68 APIs in swagger.yaml
