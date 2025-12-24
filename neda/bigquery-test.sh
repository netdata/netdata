#!/usr/bin/env bash
#
# Validation harness for bigquery.ai against KPI reference SQL.
# Runs the canonical BigQuery SQL and the agent with the matching question,
# then compares outputs numerically. Add more cases below.
#
# Requirements:
#   - bq CLI
#   - jq
#   - GOOGLE_APPLICATION_CREDENTIALS and BIGQUERY_PROJECT set
#   - repo-local `./ai-agent` (or set AI_AGENT_BIN) + `neda/bigquery.ai`
#
# Usage:
#   ./neda/bigquery-test.sh          # run all cases with default last 7d window
#   FROM_DATE=2025-12-10 TO_DATE=2025-12-17 ./neda/bigquery-test.sh  # override window
#   ./neda/bigquery-test.sh --continue            # run all cases, summarize failures
#   ./neda/bigquery-test.sh --fail-fast           # stop on first failure (default)
#   ./neda/bigquery-test.sh --jobs 3              # run up to 3 cases in parallel
#   ./neda/bigquery-test.sh --only-case realized_arr  # run a single case
#   MODEL_OVERRIDE=nova/gpt-oss-20b ./neda/bigquery-test.sh  # override model used by ai-agent
#   TEMPERATURE_OVERRIDE=0 ./neda/bigquery-test.sh          # override temperature (empty to disable)
#   LLM_TIMEOUT_MS=36000000 TOOL_TIMEOUT_MS=36000000 ./neda/bigquery-test.sh  # override timeouts (default 10h)
#
set -euo pipefail

# Transparent execution helper (from project instructions)
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; GRAY='\033[0;90m'; NC='\033[0m'
run() {
  printf >&2 "${GRAY}$(pwd) >${NC} ${YELLOW}%q " "$@"; printf >&2 "${NC}\n"
  "$@"
  local code=$?
  if [[ $code -ne 0 ]]; then
    echo -e >&2 "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e >&2 "${RED}[ERROR]${NC} Command failed with exit code ${code}: ${YELLOW}$1${NC}"
    echo -e >&2 "${RED}        Full command:${NC} $*"
    echo -e >&2 "${RED}        Working dir:${NC} $(pwd)"
    echo -e >&2 "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    return $code
  fi
}

# Resolve paths relative to this script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Basic deps
require_cmd() {
  local cmd="$1"
  if ! command -v "${cmd}" >/dev/null 2>&1; then
    echo "Missing required command: ${cmd}" >&2
    exit 1
  fi
}
require_cmd bq
require_cmd jq

# Guard env, allow defaults based on repo layout
GOOGLE_APPLICATION_CREDENTIALS="${GOOGLE_APPLICATION_CREDENTIALS:-${SCRIPT_DIR}/.neda--netdata-analytics-bi--service-account.json}"
BIGQUERY_PROJECT="${BIGQUERY_PROJECT:-netdata-analytics-bi}"

export GOOGLE_APPLICATION_CREDENTIALS BIGQUERY_PROJECT

if [[ ! -f "${GOOGLE_APPLICATION_CREDENTIALS}" ]]; then
  echo "Missing credentials file: ${GOOGLE_APPLICATION_CREDENTIALS}" >&2
  exit 1
fi

# Default: last 7 complete days (ending yesterday) for stability, matching Grafana patterns.
FROM_DATE="${FROM_DATE:-$(date -I -d '7 days ago')}"
TO_DATE="${TO_DATE:-$(date -I -d '1 day ago')}"

AI_AGENT_BIN="${AI_AGENT_BIN:-${SCRIPT_DIR}/../ai-agent}"
SYSTEM_PROMPT="${SYSTEM_PROMPT:-${SCRIPT_DIR}/bigquery.ai}"
MODEL_OVERRIDE="${MODEL_OVERRIDE:-vllm3/gpt-oss-20b}"
TEMPERATURE_OVERRIDE="${TEMPERATURE_OVERRIDE:-0}"
LLM_TIMEOUT_MS="${LLM_TIMEOUT_MS:-36000000}"
TOOL_TIMEOUT_MS="${TOOL_TIMEOUT_MS:-36000000}"

OUT_DIR="${OUT_DIR:-${SCRIPT_DIR}/../tmp/bigquery-tests}"
run mkdir -p "${OUT_DIR}"
LOG_DIR="${LOG_DIR:-${OUT_DIR}/logs}"
run mkdir -p "${LOG_DIR}"

FAIL_FAST="${FAIL_FAST:-1}"
JOBS="${JOBS:-1}"
CASE_FILTER="${ONLY_CASE:-}"
CASE_MATCHED=0

usage() {
  cat <<'EOF'
Usage: ./neda/bigquery-test.sh [--continue|--fail-fast] [--jobs N] [--only-case NAME]

  --continue        Run all cases and summarize failures (exit 1 if any failed)
  --fail-fast       Stop on first failure (default)
  --jobs N          Run up to N cases in parallel (default: 1)
  --only-case NAME  Run a single case by name (same as ONLY_CASE env)
  MODEL_OVERRIDE    Override model used by ai-agent (default: nova/gpt-oss-20b; set empty to disable)
  TEMPERATURE_OVERRIDE  Override temperature (default: 0; set empty to disable)
  LLM_TIMEOUT_MS    Override LLM timeout in ms (default: 36000000)
  TOOL_TIMEOUT_MS   Override tool timeout in ms (default: 36000000)
  -h, --help        Show help
EOF
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --continue)
        FAIL_FAST=0
        ;;
      --fail-fast)
        FAIL_FAST=1
        ;;
      -j|--jobs)
        JOBS="${2:-}"
        shift
        ;;
      --only-case)
        CASE_FILTER="${2:-}"
        shift
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        echo "Unknown argument: $1" >&2
        usage >&2
        exit 1
        ;;
    esac
    shift
  done
}

parse_args "$@"

if ! [[ "${JOBS}" =~ ^[0-9]+$ ]] || [[ "${JOBS}" -lt 1 ]]; then
  echo "Invalid --jobs value: ${JOBS} (must be >= 1)" >&2
  exit 1
fi

should_run() {
  local name="$1"
  if [[ -z "${CASE_FILTER}" ]]; then
    return 0
  fi
  if [[ "${name}" == "${CASE_FILTER}" ]]; then
    CASE_MATCHED=1
    return 0
  fi
  return 1
}

case_realized_arr_sql() {
  cat <<'SQL'
WITH dates AS (
  SELECT date
  FROM UNNEST(GENERATE_DATE_ARRAY(@from_date, @to_date)) AS date
),
realized AS (
  SELECT
    run_date AS date,
    (
      MAX(IF(metric_name = 'arr_business_discount', metric_value, NULL))
      + COALESCE(MAX(IF(metric_name = 'arr_homelab_discount', metric_value, NULL)), 0)
      + COALESCE(MAX(IF(metric_name = 'ai_credits_space_revenue', metric_value, NULL)), 0)
      + COALESCE(MAX(IF(metric_name = 'onprem_arr', metric_value, NULL)), 0)
    ) AS total_arr_discounted
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
    AND run_date > DATE '2023-11-28'
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
),
manual_total AS (
  -- Static baseline from the first on-prem snapshot table.
  -- Applied to every date <= 2025-10-01 (dates prior to 2025-10-02).
  SELECT IFNULL(SUM(annual_price), 0) AS manual_total
  FROM `netdata-analytics-bi.watch_towers.manual360_asat_20251002`
  WHERE expiry_date > DATE '2025-10-01'
    AND start_date <= DATE '2025-10-01'
),
combined AS (
  SELECT
    d.date,
    COALESCE(r.total_arr_discounted, 0) AS total_arr_discounted,
    CASE
      WHEN d.date <= DATE '2025-10-01' THEN m.manual_total
      ELSE 0
    END AS manual_onprem_static
  FROM dates d
  LEFT JOIN realized r ON d.date = r.date
  CROSS JOIN manual_total m
)
SELECT
  date,
  total_arr_discounted + COALESCE(manual_onprem_static, 0) AS realized_arr
FROM combined
WHERE date < CURRENT_DATE()
ORDER BY date;
SQL
}

case_realized_arr_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": [
          "date",
          "realized_arr"
        ],
        "properties": {
          "date": {
            "type": "string"
          },
          "realized_arr": {
            "type": "number"
          }
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_realized_arr_components_sql() {
  cat <<'SQL'
WITH dates AS (
  SELECT date
  FROM UNNEST(GENERATE_DATE_ARRAY(@from_date, @to_date)) AS date
),
realized AS (
  SELECT
    run_date AS date,
    MAX(IF(metric_name = 'arr_business_discount', metric_value, NULL)) AS business,
    MAX(IF(metric_name = 'onprem_arr', metric_value, NULL)) AS onprem_arr,
    MAX(IF(metric_name = 'arr_homelab_discount', metric_value, NULL)) AS homelab,
    MAX(IF(metric_name = 'ai_credits_space_revenue', metric_value, NULL)) AS ai_bundles
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
    AND run_date > DATE '2023-11-28'
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
),
manual_total AS (
  -- single snapshot sum; applied to every date <= 2025-10-01
  SELECT IFNULL(SUM(annual_price), 0) AS manual_total
  FROM `netdata-analytics-bi.watch_towers.manual360_asat_20251002`
  WHERE expiry_date > DATE '2025-10-01'
    AND start_date <= DATE '2025-10-01'
),
combined AS (
  SELECT
    d.date,
    COALESCE(r.business, 0) AS business,
    COALESCE(r.homelab, 0) AS homelab,
    COALESCE(r.ai_bundles, 0) AS ai_bundles,
    COALESCE(r.onprem_arr, 0) + CASE WHEN d.date <= DATE '2025-10-01' THEN m.manual_total ELSE 0 END AS on_prem
  FROM dates d
  LEFT JOIN realized r ON d.date = r.date
  CROSS JOIN manual_total m
)
SELECT
  date,
  on_prem,
  business,
  homelab,
  ai_bundles,
  on_prem + business + homelab + ai_bundles AS total
FROM combined
WHERE date < CURRENT_DATE()
ORDER BY date;
SQL
}

case_realized_arr_components_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": [
          "date",
          "on_prem",
          "business",
          "homelab",
          "ai_bundles",
          "total"
        ],
        "properties": {
          "date": {
            "type": "string"
          },
          "on_prem": {
            "type": "number"
          },
          "business": {
            "type": "number"
          },
          "homelab": {
            "type": "number"
          },
          "ai_bundles": {
            "type": "number"
          },
          "total": {
            "type": "number"
          }
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_realized_arr_percent_sql() {
  cat <<'SQL'
WITH dates AS (
  SELECT date
  FROM UNNEST(GENERATE_DATE_ARRAY(@from_date, @to_date)) AS date
),
realized AS (
  SELECT
    run_date AS date,
    MAX(IF(metric_name = 'arr_business_discount', metric_value, NULL)) AS business,
    MAX(IF(metric_name = 'onprem_arr', metric_value, NULL)) AS onprem_arr,
    MAX(IF(metric_name = 'arr_homelab_discount', metric_value, NULL)) AS homelab,
    MAX(IF(metric_name = 'ai_credits_space_revenue', metric_value, NULL)) AS ai_bundles
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
    AND run_date > DATE '2023-11-28'
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
),
manual_total AS (
  SELECT IFNULL(SUM(annual_price), 0) AS manual_total
  FROM `netdata-analytics-bi.watch_towers.manual360_asat_20251002`
  WHERE expiry_date > DATE '2025-10-01'
    AND start_date <= DATE '2025-10-01'
),
main AS (
  SELECT
    d.date,
    COALESCE(r.business, 0) AS business,
    COALESCE(r.homelab, 0) AS homelab,
    COALESCE(r.ai_bundles, 0) AS ai_bundles,
    COALESCE(r.onprem_arr, 0) + CASE WHEN d.date <= DATE '2025-10-01' THEN m.manual_total ELSE 0 END AS on_prem_total,
    (
      COALESCE(r.onprem_arr, 0)
      + CASE WHEN d.date <= DATE '2025-10-01' THEN m.manual_total ELSE 0 END
      + COALESCE(r.business, 0)
      + COALESCE(r.homelab, 0)
      + COALESCE(r.ai_bundles, 0)
    ) AS total_arr
  FROM dates d
  LEFT JOIN realized r ON d.date = r.date
  CROSS JOIN manual_total m
)
SELECT
  date,
  SAFE_DIVIDE(on_prem_total, total_arr) * 100 AS on_prem,
  SAFE_DIVIDE(business, total_arr) * 100 AS business,
  SAFE_DIVIDE(homelab, total_arr) * 100 AS homelab,
  SAFE_DIVIDE(ai_bundles, total_arr) * 100 AS ai_bundles,
  SAFE_DIVIDE(on_prem_total, total_arr) * 100
    + SAFE_DIVIDE(business, total_arr) * 100
    + SAFE_DIVIDE(homelab, total_arr) * 100
    + SAFE_DIVIDE(ai_bundles, total_arr) * 100 AS total
FROM main
WHERE date < CURRENT_DATE()
ORDER BY date;
SQL
}

case_realized_arr_percent_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": [
          "date",
          "on_prem",
          "business",
          "homelab",
          "ai_bundles",
          "total"
        ],
        "properties": {
          "date": {
            "type": "string"
          },
          "on_prem": {
            "type": "number"
          },
          "business": {
            "type": "number"
          },
          "homelab": {
            "type": "number"
          },
          "ai_bundles": {
            "type": "number"
          },
          "total": {
            "type": "number"
          }
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_trials_total_last_sql() {
  cat <<'SQL'
WITH series AS (
  SELECT
    run_date AS date,
    MAX(IF(metric_name = 'trials_total', metric_value, NULL)) AS trials_total
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
)
SELECT
  date,
  trials_total
FROM series
WHERE trials_total IS NOT NULL
ORDER BY date DESC
LIMIT 1;
SQL
}

case_trials_total_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "date",
        "trials_total"
      ],
      "properties": {
        "date": {
          "type": "string"
        },
        "trials_total": {
          "type": "number"
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_trial_6plus_nodes_est_value_sql() {
  cat <<'SQL'
WITH series AS (
  SELECT
    run_date AS date,
    MAX(IF(metric_name = 'trial_6_or_more_nodes_reachable_sum', metric_value, NULL)) * 2 * 12 AS est_value
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
)
SELECT
  date,
  est_value
FROM series
WHERE est_value IS NOT NULL
ORDER BY date DESC
LIMIT 1;
SQL
}

case_trial_6plus_nodes_est_value_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": ["data", "notes", "data_freshness"],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": ["date", "est_value"],
      "properties": {
        "date": { "type": "string" },
        "est_value": { "type": "number" }
      }
    },
    "notes": {
      "type": "array",
      "items": { "type": "string" }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": ["last_ingested_at", "age_minutes", "source_table"],
      "properties": {
        "last_ingested_at": { "type": "string" },
        "age_minutes": { "type": "number" },
        "source_table": { "type": "string" }
      }
    }
  }
}
JSON
}

# Growth metrics (discounted ARR % growth)
case_arr_discounted_growth_pct_sql() {
  cat <<'SQL'
WITH series AS (
  SELECT
    run_date AS date,
    (
      MAX(IF(metric_name = 'arr_business_discount', metric_value, NULL))
      + COALESCE(MAX(IF(metric_name = 'arr_homelab_discount', metric_value, NULL)), 0)
      + COALESCE(MAX(IF(metric_name = 'ai_credits_space_revenue', metric_value, NULL)), 0)
      + COALESCE(MAX(IF(metric_name = 'onprem_arr', metric_value, NULL)), 0)
    ) AS metric
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
    AND run_date > DATE '2023-11-28'
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
)
SELECT
  date,
  SAFE_DIVIDE(metric - LAG(metric, 7) OVER (ORDER BY date), LAG(metric, 7) OVER (ORDER BY date)) * 100 AS pct_7,
  SAFE_DIVIDE(metric - LAG(metric, 30) OVER (ORDER BY date), LAG(metric, 30) OVER (ORDER BY date)) * 100 AS pct_30,
  SAFE_DIVIDE(metric - LAG(metric, 90) OVER (ORDER BY date), LAG(metric, 90) OVER (ORDER BY date)) * 100 AS pct_90
FROM series
WHERE date < CURRENT_DATE()
ORDER BY date;
SQL
}

case_arr_growth_pct_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": ["data", "notes", "data_freshness"],
  "properties": {
    "data": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["date", "pct_7", "pct_30", "pct_90"],
        "properties": {
          "date": { "type": "string" },
          "pct_7": { "type": ["number", "null"] },
          "pct_30": { "type": ["number", "null"] },
          "pct_90": { "type": ["number", "null"] }
        }
      }
    },
    "notes": { "type": "array", "items": { "type": "string" } },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": ["last_ingested_at", "age_minutes", "source_table"],
      "properties": {
        "last_ingested_at": { "type": "string" },
        "age_minutes": { "type": "number" },
        "source_table": { "type": "string" }
      }
    }
  }
}
JSON
}

# Growth metrics (undiscounted ARR % growth)
case_arr_undiscounted_growth_pct_sql() {
  cat <<'SQL'
WITH series AS (
  SELECT
    run_date AS date,
    (
      MAX(IF(metric_name = 'arr_business', metric_value, NULL))
      + COALESCE(MAX(IF(metric_name = 'arr_homelab', metric_value, NULL)), 0)
      + COALESCE(MAX(IF(metric_name = 'ai_credits_space_revenue', metric_value, NULL)), 0)
      + COALESCE(MAX(IF(metric_name = 'onprem_arr', metric_value, NULL)), 0)
    ) AS metric
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
    AND run_date > DATE '2023-11-28'
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
)
SELECT
  date,
  SAFE_DIVIDE(metric - LAG(metric, 7) OVER (ORDER BY date), LAG(metric, 7) OVER (ORDER BY date)) * 100 AS pct_7,
  SAFE_DIVIDE(metric - LAG(metric, 30) OVER (ORDER BY date), LAG(metric, 30) OVER (ORDER BY date)) * 100 AS pct_30,
  SAFE_DIVIDE(metric - LAG(metric, 90) OVER (ORDER BY date), LAG(metric, 90) OVER (ORDER BY date)) * 100 AS pct_90
FROM series
WHERE date < CURRENT_DATE()
ORDER BY date;
SQL
}

# Growth metrics helpers for nodes/customers
case_metric_growth_pct_sql() {
  local metric_expr="$1"
  cat <<'SQL' | sed "s/__METRIC_EXPR__/${metric_expr}/g"
WITH series AS (
  SELECT
    run_date AS date,
    __METRIC_EXPR__ AS metric
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
)
SELECT
  date,
  SAFE_DIVIDE(metric - LAG(metric, 7) OVER (ORDER BY date), LAG(metric, 7) OVER (ORDER BY date)) * 100 AS pct_7,
  SAFE_DIVIDE(metric - LAG(metric, 30) OVER (ORDER BY date), LAG(metric, 30) OVER (ORDER BY date)) * 100 AS pct_30,
  SAFE_DIVIDE(metric - LAG(metric, 90) OVER (ORDER BY date), LAG(metric, 90) OVER (ORDER BY date)) * 100 AS pct_90
FROM series
WHERE date < CURRENT_DATE()
ORDER BY date;
SQL
}


case_business_plan_services_money_sql() {
  cat <<'SQL'
WITH series AS (
  SELECT
    run_date AS date,
    MAX(IF(metric_name = 'business_plan_services_money', metric_value, NULL)) AS business_plan_services_money
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
)
SELECT
  date,
  business_plan_services_money
FROM series
WHERE business_plan_services_money IS NOT NULL
ORDER BY date DESC
LIMIT 1;
SQL
}

case_business_plan_services_money_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "date",
        "business_plan_services_money"
      ],
      "properties": {
        "date": {
          "type": "string"
        },
        "business_plan_services_money": {
          "type": "number"
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_total_arr_plus_unrealized_arr_sql() {
  cat <<'SQL'
WITH dates AS (
  SELECT date
  FROM UNNEST(GENERATE_DATE_ARRAY(@from_date, @to_date)) AS date
),
daily AS (
  SELECT
    run_date AS date,
    (
      MAX(IF(metric_name = 'arr_business_discount', metric_value, NULL))
      + COALESCE(MAX(IF(metric_name = 'arr_homelab_discount', metric_value, NULL)), 0)
      + COALESCE(MAX(IF(metric_name = 'business_45d_monthly_newcomer_arr', metric_value, NULL)), 0)
      + COALESCE(MAX(IF(metric_name = 'homelab_45d_monthly_newcomer_arr', metric_value, NULL)), 0)
      + COALESCE(MAX(IF(metric_name = 'ai_credits_space_revenue', metric_value, NULL)), 0)
      + COALESCE(MAX(IF(metric_name = 'onprem_arr', metric_value, NULL)), 0)
      + COALESCE(MAX(IF(metric_name = 'business_overrides_arr', metric_value, NULL)), 0)
    ) AS total_arr_discounted
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
    AND run_date > DATE '2023-11-28'
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
),
manual_total AS (
  SELECT IFNULL(SUM(annual_price), 0) AS manual_total
  FROM `netdata-analytics-bi.watch_towers.manual360_asat_20251002`
  WHERE expiry_date > DATE '2025-10-01'
    AND start_date <= DATE '2025-10-01'
),
combined AS (
  SELECT
    d.date,
    COALESCE(x.total_arr_discounted, 0) AS total_arr_discounted,
    CASE
      WHEN d.date <= DATE '2025-10-01' THEN m.manual_total
      ELSE 0
    END AS manual_onprem_static
  FROM dates d
  LEFT JOIN daily x ON d.date = x.date
  CROSS JOIN manual_total m
)
SELECT
  date,
  total_arr_discounted + COALESCE(manual_onprem_static, 0) AS total_arr_discounted
FROM combined
WHERE date < CURRENT_DATE()
ORDER BY date DESC
LIMIT 1;
SQL
}

case_total_arr_plus_unrealized_arr_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "date",
        "total_arr_discounted"
      ],
      "properties": {
        "date": {
          "type": "string"
        },
        "total_arr_discounted": {
          "type": "number"
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_unrealized_arr_sql() {
  cat <<'SQL'
WITH series AS (
  SELECT
    run_date AS date,
    MAX(IF(metric_name = 'business_45d_monthly_newcomer_arr', metric_value, NULL)) AS business,
    MAX(IF(metric_name = 'homelab_45d_monthly_newcomer_arr', metric_value, NULL)) AS homelab
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
  GROUP BY run_date
)
SELECT
  date,
  business,
  homelab
FROM series
WHERE business IS NOT NULL OR homelab IS NOT NULL
ORDER BY date DESC
LIMIT 1;
SQL
}

case_unrealized_arr_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "date",
        "business",
        "homelab"
      ],
      "properties": {
        "date": {
          "type": "string"
        },
        "business": {
          "type": "number"
        },
        "homelab": {
          "type": "number"
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_new_business_subscriptions_sql() {
  cat <<'SQL'
SELECT
  run_date AS date,
  MAX(IF(metric_name IN ('new_subs', 'new_business_subs'), metric_value, NULL)) AS new_business_subs
FROM `netdata-analytics-bi.metrics.metrics_daily`
WHERE run_date BETWEEN @from_date AND @to_date
  AND run_date < CURRENT_DATE()
GROUP BY run_date
ORDER BY run_date;
SQL
}

case_new_business_subscriptions_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": [
          "date",
          "new_business_subs"
        ],
        "properties": {
          "date": {
            "type": "string"
          },
          "new_business_subs": {
            "type": [
              "number",
              "null"
            ]
          }
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_churned_business_subscriptions_sql() {
  cat <<'SQL'
SELECT
  run_date AS date,
  MAX(IF(metric_name IN ('churn_subs', 'churn_business_subs'), metric_value, NULL)) AS churn_business_subs
FROM `netdata-analytics-bi.metrics.metrics_daily`
WHERE run_date BETWEEN @from_date AND @to_date
  AND run_date < CURRENT_DATE()
GROUP BY run_date
ORDER BY run_date;
SQL
}

case_churned_business_subscriptions_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": [
          "date",
          "churn_business_subs"
        ],
        "properties": {
          "date": {
            "type": "string"
          },
          "churn_business_subs": {
            "type": [
              "number",
              "null"
            ]
          }
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_ai_bundle_metrics_sql() {
  cat <<'SQL'
SELECT
  run_date AS date,
  MAX(IF(metric_name = 'Bundle625.00', metric_value, NULL)) AS bundle_625,
  MAX(IF(metric_name = 'Bundle300.00', metric_value, NULL)) AS bundle_300,
  MAX(IF(metric_name = 'Bundle1100.00', metric_value, NULL)) AS bundle_1100,
  MAX(IF(metric_name = 'Bundle2000.00', metric_value, NULL)) AS bundle_2000
FROM `netdata-analytics-bi.metrics.metrics_daily`
WHERE run_date BETWEEN @from_date AND @to_date
  AND run_date < CURRENT_DATE()
GROUP BY run_date
ORDER BY run_date;
SQL
}

case_ai_bundle_metrics_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": [
          "date",
          "bundle_625",
          "bundle_300",
          "bundle_1100",
          "bundle_2000"
        ],
        "properties": {
          "date": {
            "type": "string"
          },
          "bundle_625": {
            "type": [
              "number",
              "null"
            ]
          },
          "bundle_300": {
            "type": [
              "number",
              "null"
            ]
          },
          "bundle_1100": {
            "type": [
              "number",
              "null"
            ]
          },
          "bundle_2000": {
            "type": [
              "number",
              "null"
            ]
          }
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_ai_credits_space_count_sql() {
  cat <<'SQL'
WITH series AS (
  SELECT
    run_date AS date,
    MAX(IF(metric_name = 'ai_credits_space_count', metric_value, NULL)) AS spaces
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
)
SELECT
  date,
  spaces
FROM series
WHERE spaces IS NOT NULL
ORDER BY date DESC
LIMIT 1;
SQL
}

case_ai_credits_space_count_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "date",
        "spaces"
      ],
      "properties": {
        "date": {
          "type": "string"
        },
        "spaces": {
          "type": "number"
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_business_subscriptions_sql() {
  cat <<'SQL'
WITH series AS (
  SELECT
    run_date AS date,
    MAX(IF(metric_name IN ('subscriptions_total', 'total_business_subscriptions'), metric_value, NULL)) AS cloud_subscriptions
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
  GROUP BY run_date
)
SELECT
  date,
  cloud_subscriptions
FROM series
WHERE cloud_subscriptions IS NOT NULL
ORDER BY date DESC
LIMIT 1;
SQL
}

case_business_subscriptions_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "date",
        "cloud_subscriptions"
      ],
      "properties": {
        "date": {
          "type": "string"
        },
        "cloud_subscriptions": {
          "type": "number"
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_business_arr_discount_sql() {
  cat <<'SQL'
WITH series AS (
  SELECT
    run_date AS date,
    MAX(IF(metric_name = 'arr_business_discount', metric_value, NULL)) AS discounted_arr
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
    AND run_date > DATE '2023-11-28'
  GROUP BY run_date
)
SELECT
  date,
  discounted_arr
FROM series
WHERE discounted_arr IS NOT NULL
ORDER BY date DESC
LIMIT 1;
SQL
}

case_business_arr_discount_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "date",
        "discounted_arr"
      ],
      "properties": {
        "date": {
          "type": "string"
        },
        "discounted_arr": {
          "type": "number"
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_business_nodes_sql() {
  cat <<'SQL'
WITH series AS (
  SELECT
    run_date AS date,
    MAX(IF(metric_name = 'paid_nodes_business_annual', metric_value, NULL))
      + MAX(IF(metric_name = 'paid_nodes_business_monthly', metric_value, NULL)) AS total
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
  GROUP BY run_date
)
SELECT
  date,
  total
FROM series
WHERE total IS NOT NULL
ORDER BY date DESC
LIMIT 1;
SQL
}

case_business_nodes_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "date",
        "total"
      ],
      "properties": {
        "date": {
          "type": "string"
        },
        "total": {
          "type": "number"
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_windows_reachable_nodes_sql() {
  cat <<'SQL'
WITH series AS (
  SELECT
    run_date AS date,
    MAX(IF(metric_name = 'windows_reachable_nodes', metric_value, NULL)) AS windows_nodes
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
)
SELECT
  date,
  windows_nodes
FROM series
WHERE windows_nodes IS NOT NULL
ORDER BY date DESC
LIMIT 1;
SQL
}

case_windows_reachable_nodes_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "date",
        "windows_nodes"
      ],
      "properties": {
        "date": {
          "type": "string"
        },
        "windows_nodes": {
          "type": "number"
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_homelab_subscriptions_sql() {
  cat <<'SQL'
WITH series AS (
  SELECT
    run_date AS date,
    MAX(IF(metric_name = 'total_homelab_subscriptions', metric_value, NULL)) AS homelab_subs
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
  GROUP BY run_date
)
SELECT
  date,
  homelab_subs
FROM series
WHERE homelab_subs IS NOT NULL
ORDER BY date DESC
LIMIT 1;
SQL
}

case_homelab_subscriptions_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "date",
        "homelab_subs"
      ],
      "properties": {
        "date": {
          "type": "string"
        },
        "homelab_subs": {
          "type": "number"
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_homelab_arr_discount_sql() {
  cat <<'SQL'
WITH series AS (
  SELECT
    run_date AS date,
    MAX(IF(metric_name = 'arr_homelab_discount', metric_value, NULL)) AS discounted_arr
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
  GROUP BY run_date
)
SELECT
  date,
  discounted_arr
FROM series
WHERE discounted_arr IS NOT NULL
ORDER BY date DESC
LIMIT 1;
SQL
}

case_homelab_arr_discount_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "date",
        "discounted_arr"
      ],
      "properties": {
        "date": {
          "type": "string"
        },
        "discounted_arr": {
          "type": "number"
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_homelab_nodes_sql() {
  cat <<'SQL'
WITH series AS (
  SELECT
    run_date AS date,
    MAX(IF(metric_name = 'total_reachable_nodes_homelab', metric_value, NULL)) AS total
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
  GROUP BY run_date
)
SELECT
  date,
  total
FROM series
WHERE total IS NOT NULL
ORDER BY date DESC
LIMIT 1;
SQL
}

case_homelab_nodes_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "date",
        "total"
      ],
      "properties": {
        "date": {
          "type": "string"
        },
        "total": {
          "type": "number"
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_on_prem_customers_sql() {
  cat <<'SQL'
WITH onprem_static AS (
  SELECT 5 AS onprem
),
metrics AS (
  SELECT
    run_date AS date,
    MAX(IF(metric_name = 'onprem_customers', metric_value, NULL)) AS customers
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
    AND run_date > DATE '2023-11-28'
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
),
combined AS (
  SELECT
    m.date,
    CASE
      WHEN m.date <= DATE '2025-10-04' THEN o.onprem
      ELSE m.customers
    END AS customers
  FROM metrics m
  CROSS JOIN onprem_static o
)
SELECT
  date,
  customers
FROM combined
WHERE date < CURRENT_DATE()
ORDER BY date DESC
LIMIT 1;
SQL
}

case_on_prem_customers_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "date",
        "customers"
      ],
      "properties": {
        "date": {
          "type": "string"
        },
        "customers": {
          "type": "number"
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_on_prem_arr_sql() {
  cat <<'SQL'
WITH series AS (
  SELECT
    run_date AS date,
    MAX(IF(metric_name = 'onprem_arr', metric_value, NULL)) AS onprem_arr
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
)
SELECT
  date,
  onprem_arr
FROM series
WHERE onprem_arr IS NOT NULL
ORDER BY date DESC
LIMIT 1;
SQL
}

case_on_prem_arr_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "date",
        "onprem_arr"
      ],
      "properties": {
        "date": {
          "type": "string"
        },
        "onprem_arr": {
          "type": "number"
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_windows_reachable_nodes_breakdown_sql() {
  cat <<'SQL'
SELECT
  run_date AS date,
  MAX(IF(metric_name = 'windows_reachable_nodes_business', metric_value, NULL)) AS business,
  MAX(IF(metric_name = 'windows_reachable_nodes_trial', metric_value, NULL)) AS trial,
  MAX(IF(metric_name = 'windows_reachable_nodes_community', metric_value, NULL)) AS community,
  MAX(IF(metric_name = 'windows_reachable_nodes_homelab', metric_value, NULL)) AS homelab
FROM `netdata-analytics-bi.metrics.metrics_daily`
WHERE run_date BETWEEN @from_date AND @to_date
  AND run_date < CURRENT_DATE()
GROUP BY run_date
ORDER BY run_date;
SQL
}

case_windows_reachable_nodes_breakdown_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": [
          "date",
          "business",
          "trial",
          "community",
          "homelab"
        ],
        "properties": {
          "date": {
            "type": "string"
          },
          "business": {
            "type": [
              "number",
              "null"
            ]
          },
          "trial": {
            "type": [
              "number",
              "null"
            ]
          },
          "community": {
            "type": [
              "number",
              "null"
            ]
          },
          "homelab": {
            "type": [
              "number",
              "null"
            ]
          }
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_realized_arr_stat_sql() {
  cat <<'SQL'
WITH realized AS (
  SELECT
    run_date AS date,
    (
      MAX(IF(metric_name = 'arr_business_discount', metric_value, NULL))
      + COALESCE(MAX(IF(metric_name = 'arr_homelab_discount', metric_value, NULL)), 0)
      + COALESCE(MAX(IF(metric_name = 'ai_credits_space_revenue', metric_value, NULL)), 0)
      + COALESCE(MAX(IF(metric_name = 'onprem_arr', metric_value, NULL)), 0)
    ) AS total_arr_discounted
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
    AND run_date > DATE '2023-11-28'
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
),
manual_total AS (
  SELECT IFNULL(SUM(annual_price), 0) AS manual_total
  FROM `netdata-analytics-bi.watch_towers.manual360_asat_20251002`
  WHERE expiry_date > DATE '2025-10-01'
    AND start_date <= DATE '2025-10-01'
),
combined AS (
  SELECT
    r.date,
    COALESCE(r.total_arr_discounted, 0)
      + CASE WHEN r.date <= DATE '2025-10-01' THEN m.manual_total ELSE 0 END AS realized_arr
  FROM realized r
  CROSS JOIN manual_total m
)
SELECT
  date,
  realized_arr
FROM combined
ORDER BY date DESC
LIMIT 1;
SQL
}

case_realized_arr_stat_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "date",
        "realized_arr"
      ],
      "properties": {
        "date": {
          "type": "string"
        },
        "realized_arr": {
          "type": "number"
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_realized_arr_deltas_sql() {
  cat <<'SQL'
WITH dates AS (
  SELECT d AS date
  FROM UNNEST(GENERATE_DATE_ARRAY(DATE_SUB(@to_date, INTERVAL 90 DAY), @to_date)) AS d
),
realized AS (
  SELECT
    run_date AS date,
    (
      MAX(IF(metric_name = 'arr_business_discount', metric_value, NULL))
      + COALESCE(MAX(IF(metric_name = 'arr_homelab_discount', metric_value, NULL)), 0)
      + COALESCE(MAX(IF(metric_name = 'ai_credits_space_revenue', metric_value, NULL)), 0)
      + COALESCE(MAX(IF(metric_name = 'onprem_arr', metric_value, NULL)), 0)
    ) AS total_arr_discounted
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN DATE_SUB(@to_date, INTERVAL 90 DAY) AND @to_date
    AND run_date > DATE '2023-11-28'
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
),
manual_total AS (
  SELECT IFNULL(SUM(annual_price), 0) AS manual_total
  FROM `netdata-analytics-bi.watch_towers.manual360_asat_20251002`
  WHERE expiry_date > DATE '2025-10-01'
    AND start_date <= DATE '2025-10-01'
),
combined AS (
  SELECT
    d.date,
    COALESCE(r.total_arr_discounted, 0)
      + CASE WHEN d.date <= DATE '2025-10-01' THEN m.manual_total ELSE 0 END AS realized_arr
  FROM dates d
  LEFT JOIN realized r ON d.date = r.date
  CROSS JOIN manual_total m
),
deltas AS (
  SELECT
    date,
    (realized_arr - LAG(realized_arr, 7) OVER (ORDER BY date)) AS last_7_days,
    (realized_arr - LAG(realized_arr, 30) OVER (ORDER BY date)) AS last_30_days,
    (realized_arr - LAG(realized_arr, 90) OVER (ORDER BY date)) AS last_90_days
  FROM combined
)
SELECT
  date,
  last_7_days,
  last_30_days,
  last_90_days
FROM deltas
WHERE last_7_days IS NOT NULL AND last_30_days IS NOT NULL AND last_90_days IS NOT NULL
ORDER BY date DESC
LIMIT 1;
SQL
}

case_realized_arr_deltas_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "date",
        "last_7_days",
        "last_30_days",
        "last_90_days"
      ],
      "properties": {
        "date": {
          "type": "string"
        },
        "last_7_days": {
          "type": "number"
        },
        "last_30_days": {
          "type": "number"
        },
        "last_90_days": {
          "type": "number"
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_business_subscriptions_deltas_sql() {
  cat <<'SQL'
WITH series AS (
  SELECT
    run_date AS date,
    MAX(IF(metric_name IN ('subscriptions_total', 'total_business_subscriptions'), metric_value, NULL)) AS cloud_subscriptions
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date <= @to_date
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
  ORDER BY run_date ASC
),
deltas AS (
  SELECT
    date,
    (cloud_subscriptions - LAG(cloud_subscriptions, 7) OVER (ORDER BY date)) AS last_7_days,
    (cloud_subscriptions - LAG(cloud_subscriptions, 30) OVER (ORDER BY date)) AS last_30_days,
    (cloud_subscriptions - LAG(cloud_subscriptions, 90) OVER (ORDER BY date)) AS last_90_days
  FROM series
)
SELECT
  date,
  last_7_days,
  last_30_days,
  last_90_days
FROM deltas
ORDER BY date DESC
LIMIT 1;
SQL
}

case_business_subscriptions_deltas_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "date",
        "last_7_days",
        "last_30_days",
        "last_90_days"
      ],
      "properties": {
        "date": {
          "type": "string"
        },
        "last_7_days": {
          "type": [
            "number",
            "null"
          ]
        },
        "last_30_days": {
          "type": [
            "number",
            "null"
          ]
        },
        "last_90_days": {
          "type": [
            "number",
            "null"
          ]
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_business_arr_discount_deltas_sql() {
  cat <<'SQL'
WITH series AS (
  SELECT
    run_date AS date,
    MAX(IF(metric_name = 'arr_business_discount', metric_value, NULL)) AS discounted_arr
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date <= @to_date
    AND run_date > DATE '2023-11-28'
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
  ORDER BY run_date ASC
),
deltas AS (
  SELECT
    date,
    (discounted_arr - LAG(discounted_arr, 7) OVER (ORDER BY date)) AS last_7_days,
    (discounted_arr - LAG(discounted_arr, 30) OVER (ORDER BY date)) AS last_30_days,
    (discounted_arr - LAG(discounted_arr, 90) OVER (ORDER BY date)) AS last_90_days
  FROM series
)
SELECT
  date,
  last_7_days,
  last_30_days,
  last_90_days
FROM deltas
ORDER BY date DESC
LIMIT 1;
SQL
}

case_business_arr_discount_deltas_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "date",
        "last_7_days",
        "last_30_days",
        "last_90_days"
      ],
      "properties": {
        "date": {
          "type": "string"
        },
        "last_7_days": {
          "type": [
            "number",
            "null"
          ]
        },
        "last_30_days": {
          "type": [
            "number",
            "null"
          ]
        },
        "last_90_days": {
          "type": [
            "number",
            "null"
          ]
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_business_nodes_deltas_sql() {
  cat <<'SQL'
WITH series AS (
  SELECT
    run_date AS date,
    MAX(IF(metric_name = 'paid_nodes_business_annual', metric_value, NULL))
      + MAX(IF(metric_name = 'paid_nodes_business_monthly', metric_value, NULL)) AS total
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date <= @to_date
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
  ORDER BY run_date ASC
),
deltas AS (
  SELECT
    date,
    (total - LAG(total, 7) OVER (ORDER BY date)) AS last_7_days,
    (total - LAG(total, 30) OVER (ORDER BY date)) AS last_30_days,
    (total - LAG(total, 90) OVER (ORDER BY date)) AS last_90_days
  FROM series
)
SELECT
  date,
  last_7_days,
  last_30_days,
  last_90_days
FROM deltas
ORDER BY date DESC
LIMIT 1;
SQL
}

case_business_nodes_deltas_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "date",
        "last_7_days",
        "last_30_days",
        "last_90_days"
      ],
      "properties": {
        "date": {
          "type": "string"
        },
        "last_7_days": {
          "type": [
            "number",
            "null"
          ]
        },
        "last_30_days": {
          "type": [
            "number",
            "null"
          ]
        },
        "last_90_days": {
          "type": [
            "number",
            "null"
          ]
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_saas_spaces_counts_snapshot_sql() {
  cat <<'SQL'
SELECT
  Business,
  `Business_<45d`,
  Homelab,
  `Homelab_<45d`,
  Trials_0_nodes,
  `Trials_1-5_nodes`,
  `Trials_6+_nodes`,
  Community_0_nodes,
  Community_w_nodes,
  Total
FROM (
  SELECT
    SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class = "Business", 1, 0)) AS Business,
    SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class LIKE "Business_%", 1, 0)) AS `Business_<45d`,
    SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class = "Homelab", 1, 0)) AS Homelab,
    SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class LIKE "Homelab_%", 1, 0)) AS `Homelab_<45d`,
    SUM(IF(ax_trial_ends_at IS NOT NULL AND COALESCE(ae_reachable_nodes, 0) < 1, 1, 0)) AS Trials_0_nodes,
    SUM(IF(ax_trial_ends_at IS NOT NULL AND COALESCE(ae_reachable_nodes, 0) >= 1 AND COALESCE(ae_reachable_nodes, 0) <= 5, 1, 0)) AS `Trials_1-5_nodes`,
    SUM(IF(ax_trial_ends_at IS NOT NULL AND COALESCE(ae_reachable_nodes, 0) >= 6, 1, 0)) AS `Trials_6+_nodes`,
    SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class = "Community" AND COALESCE(ae_reachable_nodes, 0) < 1, 1, 0)) AS Community_0_nodes,
    SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class = "Community" AND COALESCE(ae_reachable_nodes, 0) >= 1, 1, 0)) AS Community_w_nodes,
    COUNT(*) AS Total
  FROM `netdata-analytics-bi.watch_towers.spaces_asat_*`
  WHERE _TABLE_SUFFIX = FORMAT_DATE('%Y%m%d', @to_date)
);
SQL
}

case_saas_spaces_counts_snapshot_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "Business",
        "Business_<45d",
        "Homelab",
        "Homelab_<45d",
        "Trials_0_nodes",
        "Trials_1-5_nodes",
        "Trials_6+_nodes",
        "Community_0_nodes",
        "Community_w_nodes",
        "Total"
      ],
      "properties": {
        "Business": {
          "type": "number"
        },
        "Business_<45d": {
          "type": "number"
        },
        "Homelab": {
          "type": "number"
        },
        "Homelab_<45d": {
          "type": "number"
        },
        "Trials_0_nodes": {
          "type": "number"
        },
        "Trials_1-5_nodes": {
          "type": "number"
        },
        "Trials_6+_nodes": {
          "type": "number"
        },
        "Community_0_nodes": {
          "type": "number"
        },
        "Community_w_nodes": {
          "type": "number"
        },
        "Total": {
          "type": "number"
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_saas_spaces_percent_snapshot_sql() {
  cat <<'SQL'
WITH counts AS (
  SELECT
    SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class = "Business", 1, 0)) AS Business,
    SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class LIKE "Business_%", 1, 0)) AS `Business_<45d`,
    SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class = "Homelab", 1, 0)) AS Homelab,
    SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class LIKE "Homelab_%", 1, 0)) AS `Homelab_<45d`,
    SUM(IF(ax_trial_ends_at IS NOT NULL AND COALESCE(ae_reachable_nodes, 0) < 1, 1, 0)) AS Trials_0_nodes,
    SUM(IF(ax_trial_ends_at IS NOT NULL AND COALESCE(ae_reachable_nodes, 0) >= 1 AND COALESCE(ae_reachable_nodes, 0) <= 5, 1, 0)) AS `Trials_1-5_nodes`,
    SUM(IF(ax_trial_ends_at IS NOT NULL AND COALESCE(ae_reachable_nodes, 0) >= 6, 1, 0)) AS `Trials_6+_nodes`,
    SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class = "Community" AND COALESCE(ae_reachable_nodes, 0) < 1, 1, 0)) AS Community_0_nodes,
    SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class = "Community" AND COALESCE(ae_reachable_nodes, 0) >= 1, 1, 0)) AS Community_w_nodes,
    (
      SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class = "Business", 1, 0))
      + SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class = "Homelab", 1, 0))
      + SUM(IF(ax_trial_ends_at IS NOT NULL AND COALESCE(ae_reachable_nodes, 0) < 1, 1, 0))
      + SUM(IF(ax_trial_ends_at IS NOT NULL AND COALESCE(ae_reachable_nodes, 0) >= 1 AND COALESCE(ae_reachable_nodes, 0) <= 5, 1, 0))
      + SUM(IF(ax_trial_ends_at IS NOT NULL AND COALESCE(ae_reachable_nodes, 0) >= 6, 1, 0))
      + SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class = "Community" AND COALESCE(ae_reachable_nodes, 0) < 1, 1, 0))
      + SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class = "Community" AND COALESCE(ae_reachable_nodes, 0) >= 1, 1, 0))
      + SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class LIKE "Business_%", 1, 0))
      + SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class LIKE "Homelab_%", 1, 0))
    ) AS calculated_total,
    COUNT(*) AS total
  FROM `netdata-analytics-bi.watch_towers.spaces_asat_*`
  WHERE _TABLE_SUFFIX = FORMAT_DATE('%Y%m%d', @to_date)
)
SELECT
  SAFE_DIVIDE(Business, total) * 100 AS Business,
  SAFE_DIVIDE(`Business_<45d`, total) * 100 AS `Business_<45d`,
  SAFE_DIVIDE(Homelab, total) * 100 AS Homelab,
  SAFE_DIVIDE(`Homelab_<45d`, total) * 100 AS `Homelab_<45d`,
  SAFE_DIVIDE(Trials_0_nodes, total) * 100 AS Trials_0_nodes,
  SAFE_DIVIDE(`Trials_1-5_nodes`, total) * 100 AS `Trials_1-5_nodes`,
  SAFE_DIVIDE(`Trials_6+_nodes`, total) * 100 AS `Trials_6+_nodes`,
  SAFE_DIVIDE(Community_0_nodes, total) * 100 AS Community_0_nodes,
  SAFE_DIVIDE(Community_w_nodes, total) * 100 AS Community_w_nodes,
  IF(CAST(SAFE_DIVIDE(calculated_total, total) * 100 AS INT64) = 100, 0, 999999) AS check
FROM counts;
SQL
}

case_saas_spaces_percent_snapshot_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "Business",
        "Business_<45d",
        "Homelab",
        "Homelab_<45d",
        "Trials_0_nodes",
        "Trials_1-5_nodes",
        "Trials_6+_nodes",
        "Community_0_nodes",
        "Community_w_nodes",
        "check"
      ],
      "properties": {
        "Business": {
          "type": "number"
        },
        "Business_<45d": {
          "type": "number"
        },
        "Homelab": {
          "type": "number"
        },
        "Homelab_<45d": {
          "type": "number"
        },
        "Trials_0_nodes": {
          "type": "number"
        },
        "Trials_1-5_nodes": {
          "type": "number"
        },
        "Trials_6+_nodes": {
          "type": "number"
        },
        "Community_0_nodes": {
          "type": "number"
        },
        "Community_w_nodes": {
          "type": "number"
        },
        "check": {
          "type": "number"
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_nodes_total_view_percent_snapshot_sql() {
  cat <<'SQL'
WITH sums AS (
  SELECT
    SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class = "Business", ae_reachable_nodes, 0)) AS Business,
    SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class LIKE "Business_%", ae_reachable_nodes, 0)) AS `Business_<45d`,
    SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class = "Homelab", ae_reachable_nodes, 0)) AS Homelab,
    SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class LIKE "Homelab_%", ae_reachable_nodes, 0)) AS `Homelab_<45d`,
    SUM(IF(ax_trial_ends_at IS NOT NULL AND COALESCE(ae_reachable_nodes, 0) >= 1 AND COALESCE(ae_reachable_nodes, 0) <= 5, ae_reachable_nodes, 0)) AS `Trials_1-5_nodes`,
    SUM(IF(ax_trial_ends_at IS NOT NULL AND COALESCE(ae_reachable_nodes, 0) >= 6, ae_reachable_nodes, 0)) AS `Trials_6+_nodes`,
    SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class = "Community" AND COALESCE(ae_reachable_nodes, 0) >= 1, ae_reachable_nodes, 0)) AS Community_w_nodes,
    (
      SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class = "Business", ae_reachable_nodes, 0))
      + SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class = "Homelab", ae_reachable_nodes, 0))
      + SUM(IF(ax_trial_ends_at IS NOT NULL AND COALESCE(ae_reachable_nodes, 0) < 1, ae_reachable_nodes, 0))
      + SUM(IF(ax_trial_ends_at IS NOT NULL AND COALESCE(ae_reachable_nodes, 0) >= 1 AND COALESCE(ae_reachable_nodes, 0) <= 5, ae_reachable_nodes, 0))
      + SUM(IF(ax_trial_ends_at IS NOT NULL AND COALESCE(ae_reachable_nodes, 0) >= 6, ae_reachable_nodes, 0))
      + SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class = "Community" AND COALESCE(ae_reachable_nodes, 0) < 1, ae_reachable_nodes, 0))
      + SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class = "Community" AND COALESCE(ae_reachable_nodes, 0) >= 1, ae_reachable_nodes, 0))
      + SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class LIKE "Business_%", ae_reachable_nodes, 0))
      + SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class LIKE "Homelab_%", ae_reachable_nodes, 0))
    ) AS calculated_total,
    SUM(ae_reachable_nodes) AS total
  FROM `netdata-analytics-bi.watch_towers.spaces_asat_*`
  WHERE _TABLE_SUFFIX = FORMAT_DATE('%Y%m%d', @to_date)
)
SELECT
  SAFE_DIVIDE(Business, total) * 100 AS Business,
  SAFE_DIVIDE(`Business_<45d`, total) * 100 AS `Business_<45d`,
  SAFE_DIVIDE(Homelab, total) * 100 AS Homelab,
  SAFE_DIVIDE(`Homelab_<45d`, total) * 100 AS `Homelab_<45d`,
  SAFE_DIVIDE(`Trials_1-5_nodes`, total) * 100 AS `Trials_1-5_nodes`,
  SAFE_DIVIDE(`Trials_6+_nodes`, total) * 100 AS `Trials_6+_nodes`,
  SAFE_DIVIDE(Community_w_nodes, total) * 100 AS Community_w_nodes,
  IF(CAST(SAFE_DIVIDE(calculated_total, total) * 100 AS INT64) = 100, 0, 999999) AS check
FROM sums;
SQL
}

case_nodes_total_view_percent_snapshot_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "Business",
        "Business_<45d",
        "Homelab",
        "Homelab_<45d",
        "Trials_1-5_nodes",
        "Trials_6+_nodes",
        "Community_w_nodes",
        "check"
      ],
      "properties": {
        "Business": {
          "type": "number"
        },
        "Business_<45d": {
          "type": "number"
        },
        "Homelab": {
          "type": "number"
        },
        "Homelab_<45d": {
          "type": "number"
        },
        "Trials_1-5_nodes": {
          "type": "number"
        },
        "Trials_6+_nodes": {
          "type": "number"
        },
        "Community_w_nodes": {
          "type": "number"
        },
        "check": {
          "type": "number"
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_nodes_counts_snapshot_sql() {
  cat <<'SQL'
SELECT
  Business,
  `Business_<45d`,
  Homelab,
  `Homelab_<45d`,
  `Trials_1-5_nodes`,
  `Trials_6+_nodes`,
  Community_w_nodes,
  Total
FROM (
  SELECT
    SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class = "Business", ae_reachable_nodes, 0)) AS Business,
    SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class LIKE "Business_%", ae_reachable_nodes, 0)) AS `Business_<45d`,
    SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class = "Homelab", ae_reachable_nodes, 0)) AS Homelab,
    SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class LIKE "Homelab_%", ae_reachable_nodes, 0)) AS `Homelab_<45d`,
    SUM(IF(ax_trial_ends_at IS NOT NULL AND COALESCE(ae_reachable_nodes, 0) >= 1 AND COALESCE(ae_reachable_nodes, 0) <= 5, ae_reachable_nodes, 0)) AS `Trials_1-5_nodes`,
    SUM(IF(ax_trial_ends_at IS NOT NULL AND COALESCE(ae_reachable_nodes, 0) >= 6, ae_reachable_nodes, 0)) AS `Trials_6+_nodes`,
    SUM(IF(ax_trial_ends_at IS NULL AND ce_plan_class = "Community" AND COALESCE(ae_reachable_nodes, 0) >= 1, ae_reachable_nodes, 0)) AS Community_w_nodes,
    SUM(ae_reachable_nodes) AS Total
  FROM `netdata-analytics-bi.watch_towers.spaces_asat_*`
  WHERE _TABLE_SUFFIX = FORMAT_DATE('%Y%m%d', @to_date)
);
SQL
}

case_nodes_counts_snapshot_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "Business",
        "Business_<45d",
        "Homelab",
        "Homelab_<45d",
        "Trials_1-5_nodes",
        "Trials_6+_nodes",
        "Community_w_nodes",
        "Total"
      ],
      "properties": {
        "Business": {
          "type": "number"
        },
        "Business_<45d": {
          "type": "number"
        },
        "Homelab": {
          "type": "number"
        },
        "Homelab_<45d": {
          "type": "number"
        },
        "Trials_1-5_nodes": {
          "type": "number"
        },
        "Trials_6+_nodes": {
          "type": "number"
        },
        "Community_w_nodes": {
          "type": "number"
        },
        "Total": {
          "type": "number"
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

###############################################################################
# KPI realized_arr templates (stat/timeseries/delta/customer diff)
###############################################################################

case_realized_arr_kpi_stat_sql() {
  cat <<'SQL'
WITH daily AS (
  SELECT
    run_date AS date,
    MAX(IF(metric_name = 'arr_business_discount', metric_value, NULL)) AS business,
    MAX(IF(metric_name = 'arr_homelab_discount', metric_value, NULL)) AS homelab,
    MAX(IF(metric_name = 'ai_credits_space_revenue', metric_value, NULL)) AS ai_bundles,
    MAX(IF(metric_name = 'onprem_arr', metric_value, NULL)) AS onprem_arr
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
    AND run_date > DATE '2023-11-28'
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
),
manual_total AS (
  SELECT IFNULL(SUM(annual_price), 0) AS manual_total
  FROM `netdata-analytics-bi.watch_towers.manual360_asat_20251002`
  WHERE expiry_date > DATE '2025-10-01'
    AND start_date <= DATE '2025-10-01'
),
combined AS (
  SELECT
    d.date,
    COALESCE(business, 0) AS business,
    COALESCE(homelab, 0) AS homelab,
    COALESCE(ai_bundles, 0) AS ai_bundles,
    COALESCE(onprem_arr, 0) + CASE WHEN d.date <= DATE '2025-10-01' THEN m.manual_total ELSE 0 END AS on_prem
  FROM daily d
  CROSS JOIN manual_total m
)
SELECT
  date,
  business,
  homelab,
  ai_bundles,
  on_prem,
  business + homelab + ai_bundles + on_prem AS total
FROM combined
WHERE date < CURRENT_DATE()
ORDER BY date DESC
LIMIT 1;
SQL
}

case_realized_arr_kpi_stat_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "date",
        "business",
        "homelab",
        "ai_bundles",
        "on_prem",
        "total"
      ],
      "properties": {
        "date": {
          "type": "string"
        },
        "business": {
          "type": "number"
        },
        "homelab": {
          "type": "number"
        },
        "ai_bundles": {
          "type": "number"
        },
        "on_prem": {
          "type": "number"
        },
        "total": {
          "type": "number"
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_realized_arr_kpi_timeseries_sql() {
  cat <<'SQL'
WITH daily AS (
  SELECT
    run_date AS date,
    MAX(IF(metric_name = 'arr_business_discount', metric_value, NULL)) AS business,
    MAX(IF(metric_name = 'arr_homelab_discount', metric_value, NULL)) AS homelab,
    MAX(IF(metric_name = 'ai_credits_space_revenue', metric_value, NULL)) AS ai_bundles,
    MAX(IF(metric_name = 'onprem_arr', metric_value, NULL)) AS onprem_arr
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
    AND run_date > DATE '2023-11-28'
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
),
manual_total AS (
  SELECT IFNULL(SUM(annual_price), 0) AS manual_total
  FROM `netdata-analytics-bi.watch_towers.manual360_asat_20251002`
  WHERE expiry_date > DATE '2025-10-01'
    AND start_date <= DATE '2025-10-01'
),
combined AS (
  SELECT
    d.date,
    COALESCE(business, 0) AS business,
    COALESCE(homelab, 0) AS homelab,
    COALESCE(ai_bundles, 0) AS ai_bundles,
    COALESCE(onprem_arr, 0) + CASE WHEN d.date <= DATE '2025-10-01' THEN m.manual_total ELSE 0 END AS on_prem
  FROM daily d
  CROSS JOIN manual_total m
)
SELECT
  date,
  business,
  homelab,
  ai_bundles,
  on_prem,
  business + homelab + ai_bundles + on_prem AS total
FROM combined
WHERE date < CURRENT_DATE()
ORDER BY date;
SQL
}

case_realized_arr_kpi_timeseries_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": [
          "date",
          "business",
          "homelab",
          "ai_bundles",
          "on_prem",
          "total"
        ],
        "properties": {
          "date": {
            "type": "string"
          },
          "business": {
            "type": "number"
          },
          "homelab": {
            "type": "number"
          },
          "ai_bundles": {
            "type": "number"
          },
          "on_prem": {
            "type": "number"
          },
          "total": {
            "type": "number"
          }
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_realized_arr_kpi_delta_sql() {
  cat <<'SQL'
WITH series AS (
  SELECT
    run_date AS date,
    MAX(IF(metric_name = 'arr_business_discount', metric_value, NULL)) AS business,
    MAX(IF(metric_name = 'arr_homelab_discount', metric_value, NULL)) AS homelab,
    MAX(IF(metric_name = 'ai_credits_space_revenue', metric_value, NULL)) AS ai_bundles,
    MAX(IF(metric_name = 'onprem_arr', metric_value, NULL)) AS onprem_arr
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
    AND run_date > DATE '2023-11-28'
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
),
manual_total AS (
  SELECT IFNULL(SUM(annual_price), 0) AS manual_total
  FROM `netdata-analytics-bi.watch_towers.manual360_asat_20251002`
  WHERE expiry_date > DATE '2025-10-01'
    AND start_date <= DATE '2025-10-01'
),
with_baseline AS (
  SELECT
    s.date,
    COALESCE(business, 0) AS business,
    COALESCE(homelab, 0) AS homelab,
    COALESCE(ai_bundles, 0) AS ai_bundles,
    COALESCE(onprem_arr, 0) + CASE WHEN s.date <= DATE '2025-10-01' THEN m.manual_total ELSE 0 END AS on_prem
  FROM series s
  CROSS JOIN manual_total m
),
ends AS (
  SELECT
    FIRST_VALUE(date) OVER (ORDER BY date) AS start_date,
    FIRST_VALUE(business) OVER (ORDER BY date) AS business_start,
    FIRST_VALUE(homelab) OVER (ORDER BY date) AS homelab_start,
    FIRST_VALUE(ai_bundles) OVER (ORDER BY date) AS ai_bundles_start,
    FIRST_VALUE(on_prem) OVER (ORDER BY date) AS on_prem_start,
    FIRST_VALUE(date) OVER (ORDER BY date DESC) AS end_date,
    FIRST_VALUE(business) OVER (ORDER BY date DESC) AS business_end,
    FIRST_VALUE(homelab) OVER (ORDER BY date DESC) AS homelab_end,
    FIRST_VALUE(ai_bundles) OVER (ORDER BY date DESC) AS ai_bundles_end,
    FIRST_VALUE(on_prem) OVER (ORDER BY date DESC) AS on_prem_end
  FROM with_baseline
  LIMIT 1
)
SELECT
  start_date,
  end_date,
  business_start,
  business_end,
  business_end - business_start AS business_delta,
  homelab_start,
  homelab_end,
  homelab_end - homelab_start AS homelab_delta,
  ai_bundles_start,
  ai_bundles_end,
  ai_bundles_end - ai_bundles_start AS ai_bundles_delta,
  on_prem_start,
  on_prem_end,
  on_prem_end - on_prem_start AS on_prem_delta,
  (business_start + homelab_start + ai_bundles_start + on_prem_start) AS total_start,
  (business_end + homelab_end + ai_bundles_end + on_prem_end) AS total_end,
  (business_end + homelab_end + ai_bundles_end + on_prem_end)
    - (business_start + homelab_start + ai_bundles_start + on_prem_start) AS total_delta
FROM ends;
SQL
}

case_realized_arr_kpi_delta_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "start_date",
        "end_date",
        "business_start",
        "business_end",
        "business_delta",
        "homelab_start",
        "homelab_end",
        "homelab_delta",
        "ai_bundles_start",
        "ai_bundles_end",
        "ai_bundles_delta",
        "on_prem_start",
        "on_prem_end",
        "on_prem_delta",
        "total_start",
        "total_end",
        "total_delta"
      ],
      "properties": {
        "start_date": {
          "type": "string"
        },
        "end_date": {
          "type": "string"
        },
        "business_start": {
          "type": "number"
        },
        "business_end": {
          "type": "number"
        },
        "business_delta": {
          "type": "number"
        },
        "homelab_start": {
          "type": "number"
        },
        "homelab_end": {
          "type": "number"
        },
        "homelab_delta": {
          "type": "number"
        },
        "ai_bundles_start": {
          "type": "number"
        },
        "ai_bundles_end": {
          "type": "number"
        },
        "ai_bundles_delta": {
          "type": "number"
        },
        "on_prem_start": {
          "type": "number"
        },
        "on_prem_end": {
          "type": "number"
        },
        "on_prem_delta": {
          "type": "number"
        },
        "total_start": {
          "type": "number"
        },
        "total_end": {
          "type": "number"
        },
        "total_delta": {
          "type": "number"
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_realized_arr_kpi_customer_diff_sql() {
  cat <<'SQL'
WITH t0 AS (
  SELECT
    aa_space_id AS space_id,
    ab_space_name AS space_name,
    ce_plan_class AS plan_class,
    COALESCE(bq_arr_discount, 0) AS arr
  FROM `netdata-analytics-bi.watch_towers.spaces_asat_*`
  WHERE _TABLE_SUFFIX = FORMAT_DATE('%Y%m%d', @from_date)
),
t1 AS (
  SELECT
    aa_space_id AS space_id,
    ab_space_name AS space_name,
    ce_plan_class AS plan_class,
    COALESCE(bq_arr_discount, 0) AS arr
  FROM `netdata-analytics-bi.watch_towers.spaces_asat_*`
  WHERE _TABLE_SUFFIX = FORMAT_DATE('%Y%m%d', @to_date)
),
joined AS (
  SELECT
    COALESCE(t1.space_id, t0.space_id) AS space_id,
    COALESCE(t1.space_name, t0.space_name) AS space_name,
    COALESCE(t1.plan_class, t0.plan_class) AS plan_class,
    COALESCE(t0.arr, 0) AS arr_t0,
    COALESCE(t1.arr, 0) AS arr_t1
  FROM t0
  FULL OUTER JOIN t1 ON t0.space_id = t1.space_id
),
classified AS (
  SELECT
    space_id,
    space_name,
    plan_class,
    arr_t0,
    arr_t1,
    arr_t1 - arr_t0 AS delta,
    CASE
      WHEN arr_t0 = 0 AND arr_t1 > 0 THEN 'won'
      WHEN arr_t0 > 0 AND arr_t1 = 0 THEN 'lost'
      WHEN arr_t1 - arr_t0 > 0 THEN 'increase'
      WHEN arr_t1 - arr_t0 < 0 THEN 'decrease'
      ELSE 'no_change'
    END AS status
  FROM joined
  WHERE arr_t0 != arr_t1
),
positive AS (
  SELECT * FROM classified
  WHERE delta > 0
  ORDER BY delta DESC, space_id
  LIMIT 10
),
negative AS (
  SELECT * FROM classified
  WHERE delta < 0
  ORDER BY delta ASC, space_id
  LIMIT 10
)
SELECT * FROM positive
UNION ALL
SELECT * FROM negative;
SQL
}

case_realized_arr_kpi_customer_diff_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": [
          "space_id",
          "space_name",
          "plan_class",
          "arr_t0",
          "arr_t1",
          "delta",
          "status"
        ],
        "properties": {
          "space_id": {
            "type": "string"
          },
          "space_name": {
            "type": [
              "string",
              "null"
            ]
          },
          "plan_class": {
            "type": [
              "string",
              "null"
            ]
          },
          "arr_t0": {
            "type": "number"
          },
          "arr_t1": {
            "type": "number"
          },
          "delta": {
            "type": "number"
          },
          "status": {
            "type": "string"
          }
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_unrealized_arr_barchart_snapshot_sql() {
  cat <<'SQL'
SELECT
  DATE(cx_arr_realized_at) AS date,
  SUM(bq_arr_discount) AS to_be_realized
FROM `netdata-analytics-bi.watch_towers.spaces_asat_*`
WHERE _TABLE_SUFFIX = FORMAT_DATE('%Y%m%d', @to_date)
  AND ax_trial_ends_at IS NULL
  AND ce_plan_class IN ('Business_45d_monthly_newcomer', 'Homelab_45d_monthly_newcomer')
  AND bq_arr_discount > 0
GROUP BY date
ORDER BY date
LIMIT 50;
SQL
}

case_unrealized_arr_barchart_snapshot_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": [
          "date",
          "to_be_realized"
        ],
        "properties": {
          "date": {
            "type": "string"
          },
          "to_be_realized": {
            "type": "number"
          }
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_business_nodes_delta_top10_sql() {
  cat <<'SQL'
WITH t0 AS (
  SELECT
    aa_space_id AS space_id,
    ab_space_name AS space_name,
    ce_plan_class AS plan_class,
    COALESCE(ae_reachable_nodes, 0) AS nodes
  FROM `netdata-analytics-bi.watch_towers.spaces_asat_*`
  WHERE _TABLE_SUFFIX = FORMAT_DATE('%Y%m%d', @from_date)
    AND ax_trial_ends_at IS NULL
    AND (ce_plan_class = 'Business' OR aw_sub_plan IN ('Business','Business2024.03'))
),
t1 AS (
  SELECT
    aa_space_id AS space_id,
    ab_space_name AS space_name,
    ce_plan_class AS plan_class,
    COALESCE(ae_reachable_nodes, 0) AS nodes
  FROM `netdata-analytics-bi.watch_towers.spaces_asat_*`
  WHERE _TABLE_SUFFIX = FORMAT_DATE('%Y%m%d', @to_date)
    AND ax_trial_ends_at IS NULL
    AND (ce_plan_class = 'Business' OR aw_sub_plan IN ('Business','Business2024.03'))
),
joined AS (
  SELECT
    COALESCE(t1.space_id, t0.space_id) AS space_id,
    COALESCE(t1.space_name, t0.space_name) AS space_name,
    COALESCE(t1.plan_class, t0.plan_class) AS plan_class,
    COALESCE(t0.nodes, 0) AS nodes_t0,
    COALESCE(t1.nodes, 0) AS nodes_t1
  FROM t0
  FULL OUTER JOIN t1 ON t0.space_id = t1.space_id
),
classified AS (
  SELECT
    space_id,
    space_name,
    plan_class,
    nodes_t0,
    nodes_t1,
    nodes_t1 - nodes_t0 AS delta,
    CASE
      WHEN nodes_t1 - nodes_t0 > 0 THEN 'increase'
      WHEN nodes_t1 - nodes_t0 < 0 THEN 'decrease'
      ELSE 'no_change'
    END AS status
  FROM joined
  WHERE nodes_t0 != nodes_t1
),
positive AS (
  SELECT * FROM classified
  WHERE delta > 0
  ORDER BY delta DESC, space_id
  LIMIT 10
),
negative AS (
  SELECT * FROM classified
  WHERE delta < 0
  ORDER BY delta ASC, space_id
  LIMIT 10
)
SELECT * FROM positive
UNION ALL
SELECT * FROM negative;
SQL
}

case_homelab_nodes_delta_top10_sql() {
  cat <<'SQL'
WITH t0 AS (
  SELECT
    aa_space_id AS space_id,
    ab_space_name AS space_name,
    ce_plan_class AS plan_class,
    COALESCE(ae_reachable_nodes, 0) AS nodes
  FROM `netdata-analytics-bi.watch_towers.spaces_asat_*`
  WHERE _TABLE_SUFFIX = FORMAT_DATE('%Y%m%d', @from_date)
    AND ax_trial_ends_at IS NULL
    AND ce_plan_class = 'Homelab'
),
t1 AS (
  SELECT
    aa_space_id AS space_id,
    ab_space_name AS space_name,
    ce_plan_class AS plan_class,
    COALESCE(ae_reachable_nodes, 0) AS nodes
  FROM `netdata-analytics-bi.watch_towers.spaces_asat_*`
  WHERE _TABLE_SUFFIX = FORMAT_DATE('%Y%m%d', @to_date)
    AND ax_trial_ends_at IS NULL
    AND ce_plan_class = 'Homelab'
),
joined AS (
  SELECT
    COALESCE(t1.space_id, t0.space_id) AS space_id,
    COALESCE(t1.space_name, t0.space_name) AS space_name,
    COALESCE(t1.plan_class, t0.plan_class) AS plan_class,
    COALESCE(t0.nodes, 0) AS nodes_t0,
    COALESCE(t1.nodes, 0) AS nodes_t1
  FROM t0
  FULL OUTER JOIN t1 ON t0.space_id = t1.space_id
),
classified AS (
  SELECT
    space_id,
    space_name,
    plan_class,
    nodes_t0,
    nodes_t1,
    nodes_t1 - nodes_t0 AS delta,
    CASE
      WHEN nodes_t1 - nodes_t0 > 0 THEN 'increase'
      WHEN nodes_t1 - nodes_t0 < 0 THEN 'decrease'
      ELSE 'no_change'
    END AS status
  FROM joined
  WHERE nodes_t0 != nodes_t1
),
positive AS (
  SELECT * FROM classified
  WHERE delta > 0
  ORDER BY delta DESC, space_id
  LIMIT 10
),
negative AS (
  SELECT * FROM classified
  WHERE delta < 0
  ORDER BY delta ASC, space_id
  LIMIT 10
)
SELECT * FROM positive
UNION ALL
SELECT * FROM negative;
SQL
}

case_space_nodes_delta_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": [
          "space_id",
          "space_name",
          "plan_class",
          "nodes_t0",
          "nodes_t1",
          "delta",
          "status"
        ],
        "properties": {
          "space_id": {
            "type": "string"
          },
          "space_name": {
            "type": [
              "string",
              "null"
            ]
          },
          "plan_class": {
            "type": [
              "string",
              "null"
            ]
          },
          "nodes_t0": {
            "type": "number"
          },
          "nodes_t1": {
            "type": "number"
          },
          "delta": {
            "type": "number"
          },
          "status": {
            "type": "string"
          }
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

case_ending_trial_spaces_barchart_snapshot_sql() {
  cat <<'SQL'
SELECT
  DATE(ax_trial_ends_at) AS date,
  SUM(ae_reachable_nodes) AS reachable_nodes_sum
FROM `netdata-analytics-bi.watch_towers.spaces_asat_*`
WHERE _TABLE_SUFFIX = FORMAT_DATE('%Y%m%d', @to_date)
  AND ax_trial_ends_at IS NOT NULL
GROUP BY date
ORDER BY date
LIMIT 50;
SQL
}

case_ending_trial_spaces_barchart_snapshot_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": [
          "date",
          "reachable_nodes_sum"
        ],
        "properties": {
          "date": {
            "type": "string"
          },
          "reachable_nodes_sum": {
            "type": "number"
          }
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

run_bq() {
  local out_file="$1" sql="$2"
  run bq query --project_id="${BIGQUERY_PROJECT}" --nouse_legacy_sql --format=prettyjson \
    --parameter="from_date:DATE:${FROM_DATE}" \
    --parameter="to_date:DATE:${TO_DATE}" \
    "${sql}" >"${out_file}"
}

run_agent() {
  local out_file="$1" schema_file="$2" question="$3"
  local override_args=()
  if [[ -n "${MODEL_OVERRIDE}" ]]; then
    override_args=(--override "models=${MODEL_OVERRIDE}")
  fi
  if [[ -n "${TEMPERATURE_OVERRIDE}" ]]; then
    override_args+=(--override "temperature=${TEMPERATURE_OVERRIDE}")
  fi
  if [[ -n "${LLM_TIMEOUT_MS}" ]]; then
    override_args+=(--override "llmTimeout=${LLM_TIMEOUT_MS}")
  fi
  if [[ -n "${TOOL_TIMEOUT_MS}" ]]; then
    override_args+=(--override "toolTimeout=${TOOL_TIMEOUT_MS}")
  fi
  run "${AI_AGENT_BIN}" \
    --verbose \
    "${SYSTEM_PROMPT}" \
    --no-stream \
    --format json \
    --schema "@${schema_file}" \
    "${override_args[@]}" \
    "Return ONLY valid JSON matching the provided schema (no prose, no extra keys). If the schema defines data as an object, return a single object (not an array). Include data_freshness{last_ingested_at,age_minutes,source_table}. ${question}" \
    >"${out_file}"
}

compare_realized_arr_timeseries() {
  local ref_file="$1" agent_file="$2" eps="${3:-0.01}"
  jq -n --slurpfile ref "${ref_file}" --slurpfile agent "${agent_file}" --argjson eps "${eps}" '
    def to_map(arr): (arr // [])
      | map({ key: .date, value: (.realized_arr | tonumber) })
      | from_entries;
    def agent_obj:
      if ($agent | length) == 0 then
        error("agent output is empty")
      elif ($agent[0] | type) == "object" then
        $agent[0]
      elif ($agent[0] | type) == "array" then
        { data: $agent[0], notes: [] }
      elif (($agent | length) >= 2) and (($agent[0] | type) == "array") and (($agent[1] | type) == "array") then
        { data: $agent[0], notes: $agent[1] }
      else
        error("agent output must be a single object {data,notes} or two JSON values: <data-array> then <notes-array>")
      end;
    (to_map($ref[0]) as $r
      | to_map((agent_obj).data) as $a
      | ($r | keys_unsorted) as $dates
      | ($a | keys_unsorted) as $adates
      | {
          ok: (
            ($dates | length) > 0
            and ($dates == $adates)
            and ($dates | all(
              ($a[.] != null) and (((($a[.] - $r[.]) | abs)) <= $eps)
            ))
          ),
          missing_in_agent: ($dates - $adates),
          extra_in_agent: ($adates - $dates),
          diffs: ($dates | map({
            date: .,
            ref: $r[.],
            agent: $a[.],
            diff: ($a[.] - $r[.])
          }))
        }
    )
  '
}

compare_timeseries_fields() {
  local ref_file="$1" agent_file="$2" fields_json="$3" eps="${4:-0.01}"
  jq -n \
    --slurpfile ref "${ref_file}" \
    --slurpfile agent "${agent_file}" \
    --argjson fields "${fields_json}" \
    --argjson eps "${eps}" '
    def to_num($v):
      if $v == null then 0 else ($v | tonumber) end;
    def diff_ok($a; $b):
      if ($a == null) or ($b == null) then true
      else ((to_num($a) - to_num($b)) | abs) <= $eps
      end;
    def normalize_row($row):
      reduce $fields[] as $f ({}; . + { ($f): to_num($row[$f]) });
    def to_map(arr): (arr // [])
      | map({ key: (.date | tostring), value: normalize_row(.) })
      | from_entries;
    def agent_obj:
      if ($agent | length) == 0 then
        error("agent output is empty")
      elif ($agent[0] | type) == "object" then
        $agent[0]
      elif ($agent[0] | type) == "array" then
        { data: $agent[0], notes: [] }
      elif (($agent | length) >= 2) and (($agent[0] | type) == "array") and (($agent[1] | type) == "array") then
        { data: $agent[0], notes: $agent[1] }
      else
        error("agent output must be a single object {data,notes} or two JSON values: <data-array> then <notes-array>")
      end;
    def values_match($rv; $av):
      if ($rv == null and $av == null) then true
      elif ($rv == null or $av == null) then false
      else ((($av - $rv) | abs) <= $eps)
      end;
    (to_map($ref[0]) as $r
      | to_map((agent_obj).data) as $a
      | ($r | keys_unsorted) as $dates
      | ($a | keys_unsorted) as $adates
      | {
          ok: (
            ($dates | length) > 0
            and ($dates == $adates)
            and ($dates | all(. as $d |
              ($fields | all(. as $f |
                values_match($r[$d][$f]; $a[$d][$f])
              ))
            ))
          ),
          missing_in_agent: ($dates - $adates),
          extra_in_agent: ($adates - $dates),
          diffs: ($dates | map(. as $d | {
            date: $d,
            fields: ($fields | map(. as $f | {
              field: $f,
              ref: $r[$d][$f],
              agent: $a[$d][$f],
              diff: (if ($r[$d][$f] != null and $a[$d][$f] != null) then ($a[$d][$f] - $r[$d][$f]) else null end)
            }))
          }))
        }
    )
  '
}

compare_keyed_rows_fields() {
  local ref_file="$1" agent_file="$2" key_field="$3" num_fields_json="$4" str_fields_json="$5" eps="${6:-0.01}"
  jq -n \
    --slurpfile ref "${ref_file}" \
    --slurpfile agent "${agent_file}" \
    --arg key_field "${key_field}" \
    --argjson num_fields "${num_fields_json}" \
    --argjson str_fields "${str_fields_json}" \
    --argjson eps "${eps}" '
    def to_num($v):
      if $v == null then null else ($v | tonumber) end;
    def num_match($rv; $av):
      if ($rv == null and $av == null) then true
      elif ($rv == null or $av == null) then false
      else ((to_num($av) - to_num($rv)) | abs) <= $eps
      end;
    def str_match($rv; $av):
      if ($rv == null and $av == null) then true
      elif ($rv == null or $av == null) then false
      else ($rv | tostring) == ($av | tostring)
      end;
    def to_map(arr): (arr // [])
      | map({ key: (.[$key_field] | tostring), value: . })
      | from_entries;
    def agent_obj:
      if ($agent | length) == 0 then
        error("agent output is empty")
      elif ($agent[0] | type) == "object" then
        $agent[0]
      elif ($agent[0] | type) == "array" then
        { data: $agent[0], notes: [] }
      elif (($agent | length) >= 2) and (($agent[0] | type) == "array") and (($agent[1] | type) == "array") then
        { data: $agent[0], notes: $agent[1] }
      else
        error("agent output must be a single object {data,notes} or two JSON values: <data-array> then <notes-array>")
      end;
    (to_map($ref[0]) as $r
      | to_map((agent_obj).data) as $a
      | ($r | keys_unsorted) as $keys
      | ($a | keys_unsorted) as $akeys
      | {
          ok: (
            ($keys | length) > 0
            and ($keys == $akeys)
            and ($keys | all(. as $k |
              ($num_fields | all(. as $f | num_match($r[$k][$f]; $a[$k][$f])))
              and ($str_fields | all(. as $f | str_match($r[$k][$f]; $a[$k][$f])))
            ))
          ),
          missing_in_agent: ($keys - $akeys),
          extra_in_agent: ($akeys - $keys),
          diffs: ($keys | map(. as $k | {
            key: $k,
            fields: (
              ($num_fields | map(. as $f | {
                field: $f,
                ref: $r[$k][$f],
                agent: $a[$k][$f],
                diff: (if ($r[$k][$f] != null and $a[$k][$f] != null)
                  then ((to_num($a[$k][$f]) - to_num($r[$k][$f])) | tostring)
                  else null end)
              }))
              + ($str_fields | map(. as $f | {
                field: $f,
                ref: $r[$k][$f],
                agent: $a[$k][$f],
                diff: (if ($r[$k][$f] == $a[$k][$f]) then "0" else "diff" end)
              }))
            )
          }))
        }
    )
  '
}

compare_single_row_fields() {
  local ref_file="$1" agent_file="$2" fields_json="$3" eps="${4:-0.01}"
  jq -n \
    --slurpfile ref "${ref_file}" \
    --slurpfile agent "${agent_file}" \
    --argjson fields "${fields_json}" \
    --argjson eps "${eps}" '
    def agent_obj:
      if ($agent | length) == 0 then
        error("agent output is empty")
      elif ($agent[0] | type) == "object" then
        $agent[0]
      elif ($agent[0] | type) == "array" then
        { data: $agent[0], notes: [] }
      elif (($agent | length) >= 2) and (($agent[0] | type) == "array") and (($agent[1] | type) == "array") then
        { data: $agent[0], notes: $agent[1] }
      else
        error("agent output must be a single object {data,notes} or two JSON values: <data-array> then <notes-array>")
      end;
    def to_num($v):
      if $v == null then null else ($v | tonumber) end;
    def values_match($rv; $av):
      if ($rv == null and $av == null) then true
      elif ($rv == null or $av == null) then false
      else ((($av - $rv) | abs) <= $eps)
      end;
    def first_row(arr; name):
      if (arr | type) != "array" then error(name + " is not an array")
      elif (arr | length) < 1 then error(name + " has no rows")
      else arr[0]
      end;
    def row_from_data(d; name):
      if (d | type) == "object" then d
      elif (d | type) == "array" then first_row(d; name)
      else error(name + " must be object or array")
      end;
    (first_row($ref[0]; "ref") as $r
      | row_from_data((agent_obj).data; "agent.data") as $a
      | {
          ok: (
            ($r.date | tostring) == ($a.date | tostring)
            and ($fields | all(. as $f | values_match(to_num($r[$f]); to_num($a[$f]))))
          ),
          ref: $r,
          agent: $a
        }
    )
  '
}

compare_single_row_fields_no_date() {
  local ref_file="$1" agent_file="$2" fields_json="$3" eps="${4:-0.01}"
  jq -n \
    --slurpfile ref "${ref_file}" \
    --slurpfile agent "${agent_file}" \
    --argjson fields "${fields_json}" \
    --argjson eps "${eps}" '
    def agent_obj:
      if ($agent | length) == 0 then
        error("agent output is empty")
      elif ($agent[0] | type) == "object" then
        $agent[0]
      elif ($agent[0] | type) == "array" then
        { data: $agent[0], notes: [] }
      elif (($agent | length) >= 2) and (($agent[0] | type) == "array") and (($agent[1] | type) == "array") then
        { data: $agent[0], notes: $agent[1] }
      else
        error("agent output must be a single object {data,notes} or two JSON values: <data-array> then <notes-array>")
      end;
    def to_num($v):
      if $v == null then null else ($v | tonumber) end;
    def values_match($rv; $av):
      if ($rv == null and $av == null) then true
      elif ($rv == null or $av == null) then false
      else ((($av - $rv) | abs) <= $eps)
      end;
    def first_row(arr; name):
      if (arr | type) != "array" then error(name + " is not an array")
      elif (arr | length) < 1 then error(name + " has no rows")
      else arr[0]
      end;
    def row_from_data(d; name):
      if (d | type) == "object" then d
      elif (d | type) == "array" then first_row(d; name)
      else error(name + " must be object or array")
      end;
    (first_row($ref[0]; "ref") as $r
      | row_from_data((agent_obj).data; "agent.data") as $a
      | {
          ok: ($fields | all(. as $f | values_match(to_num($r[$f]); to_num($a[$f])))),
          ref: $r,
          agent: $a
        }
    )
  '
}

compare_customer_delta_fields() {
  local ref_file="$1" agent_file="$2" eps="${3:-0.01}"
  jq -n \
    --slurpfile ref "${ref_file}" \
    --slurpfile agent "${agent_file}" \
    --argjson eps "${eps}" '
    def agent_obj:
      if ($agent | length) == 0 then
        error("agent output is empty")
      elif ($agent[0] | type) == "object" then
        $agent[0]
      elif (($agent | length) >= 2) and (($agent[0] | type) == "array") and (($agent[1] | type) == "array") then
        { data: $agent[0], notes: $agent[1] }
      else
        error("agent output must be a single object {data,notes} or two JSON values: <data-array> then <notes-array>")
      end;
    def to_num($v):
      if $v == null then null else ($v | tonumber) end;
    def values_match($rv; $av):
      if ($rv == null and $av == null) then true
      elif ($rv == null or $av == null) then false
      else ((($av - $rv) | abs) <= $eps)
      end;
    def safe_diff($av; $rv):
      if ($av == null or $rv == null) then null else ($av - $rv) end;
    def normalize_row($row):
      {
        arr_t0: to_num($row.arr_t0),
        arr_t1: to_num($row.arr_t1),
        delta: to_num($row.delta),
        status: ($row.status // null)
      };
    def to_map(arr):
      (arr // [])
      | map({ key: .space_id, value: normalize_row(.) })
      | from_entries;
    (to_map($ref[0]) as $rmap
      | to_map((agent_obj).data) as $amap
      | ($rmap | keys) as $rkeys
      | ($amap | keys) as $akeys
      | ($rkeys - $akeys) as $missing
      | ($akeys - $rkeys) as $extra
      | {
          ok: (
            ($missing | length) == 0
            and ($extra | length) == 0
            and ($rkeys | all(. as $k |
              ($rmap[$k].status == $amap[$k].status)
              and values_match(to_num($rmap[$k].arr_t0); to_num($amap[$k].arr_t0))
              and values_match(to_num($rmap[$k].arr_t1); to_num($amap[$k].arr_t1))
              and values_match(to_num($rmap[$k].delta); to_num($amap[$k].delta))
            ))
          ),
          missing_in_agent: $missing,
          extra_in_agent: $extra,
          diffs: ($rkeys | map({
            space_id: .,
            ref: $rmap[.],
            agent: $amap[.],
            delta_diff: safe_diff(to_num($amap[.].delta); to_num($rmap[.].delta))
          }))
        }
    )
  '
}

compare_space_nodes_delta_fields() {
  local ref_file="$1" agent_file="$2" eps="${3:-0.01}"
  jq -n \
    --slurpfile ref "${ref_file}" \
    --slurpfile agent "${agent_file}" \
    --argjson eps "${eps}" '
    def agent_obj:
      if ($agent | length) == 0 then
        error("agent output is empty")
      elif ($agent[0] | type) == "object" then
        $agent[0]
      elif (($agent | length) >= 2) and (($agent[0] | type) == "array") and (($agent[1] | type) == "array") then
        { data: $agent[0], notes: $agent[1] }
      else
        error("agent output must be a single object {data,notes} or two JSON values: <data-array> then <notes-array>")
      end;
    def to_num($v):
      if $v == null then null else ($v | tonumber) end;
    def normalize_row($row):
      {
        nodes_t0: to_num($row.nodes_t0),
        nodes_t1: to_num($row.nodes_t1),
        delta: to_num($row.delta),
        status: ($row.status // null)
      };
    def to_map(arr):
      (arr // [])
      | map({ key: .space_id, value: normalize_row(.) })
      | from_entries;
    (to_map($ref[0]) as $rmap
      | to_map((agent_obj).data) as $amap
      | ($rmap | keys) as $rkeys
      | ($amap | keys) as $akeys
      | ($rkeys - $akeys) as $missing
      | ($akeys - $rkeys) as $extra
      | {
          ok: (
            ($missing | length) == 0
            and ($extra | length) == 0
            and ($rkeys | all(. as $k |
              ($rmap[$k].status == $amap[$k].status)
              and (to_num($amap[$k].nodes_t0) == to_num($rmap[$k].nodes_t0))
              and (to_num($amap[$k].nodes_t1) == to_num($rmap[$k].nodes_t1))
              and (to_num($amap[$k].delta) == to_num($rmap[$k].delta))
            ))
          ),
          missing_in_agent: $missing,
          extra_in_agent: $extra,
          diffs: (
            $rkeys
            | map(select(
                ($rmap[.].status != $amap[.].status)
                or (to_num($amap[.].nodes_t0) != to_num($rmap[.].nodes_t0))
                or (to_num($amap[.].nodes_t1) != to_num($rmap[.].nodes_t1))
                or (to_num($amap[.].delta) != to_num($rmap[.].delta))
              ))
            | map({
                space_id: .,
                ref: $rmap[.],
                agent: $amap[.]
              })
          )
        }
    )
  '
}

run_case_realized_arr() {
  echo -e "${GREEN}=== CASE: Realized ARR (discounted, incl. on-prem) ===${NC}"

  local case_name="realized_arr"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_realized_arr_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_realized_arr_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Give me realized ARR (discounted) per day between ${FROM_DATE} and ${TO_DATE}, including on-prem."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_realized_arr_timeseries "${ref_file}" "${agent_file}" 0.01 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_realized_arr_components() {
  echo -e "${GREEN}=== CASE: Realized ARR $ components (on-prem/business/homelab/AI/total) ===${NC}"

  local case_name="realized_arr_components"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_realized_arr_components_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_realized_arr_components_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Break down realized ARR (discounted) per day between ${FROM_DATE} and ${TO_DATE} into on_prem, business, homelab, ai_bundles, and total."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_timeseries_fields "${ref_file}" "${agent_file}" '["on_prem","business","homelab","ai_bundles","total"]' 0.01 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_realized_arr_percent() {
  echo -e "${GREEN}=== CASE: Realized ARR % (component shares) ===${NC}"

  local case_name="realized_arr_percent"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_realized_arr_percent_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_realized_arr_percent_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Show realized ARR component shares (%) per day between ${FROM_DATE} and ${TO_DATE} for on_prem, business, homelab, ai_bundles, and total %."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_timeseries_fields "${ref_file}" "${agent_file}" '["on_prem","business","homelab","ai_bundles","total"]' 0.01 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_trials_total() {
  echo -e "${GREEN}=== CASE: Trials total (stat lastNotNull) ===${NC}"

  local case_name="trials_total"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_trials_total_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_trials_total_last_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Give me the latest trials_total value within ${FROM_DATE}..${TO_DATE}."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields "${ref_file}" "${agent_file}" '["trials_total"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_trial_6plus_nodes_est_value() {
  echo -e "${GREEN}=== CASE: Trial 6+ nodes estimated value (stat lastNotNull) ===${NC}"

  local case_name="trial_6plus_nodes_est_value"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_trial_6plus_nodes_est_value_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_trial_6plus_nodes_est_value_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Give me the latest estimated value for trials with 6+ reachable nodes in ${FROM_DATE}..${TO_DATE}"
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields "${ref_file}" "${agent_file}" '["est_value"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

# Safety guard: ensure schema helper is defined before growth cases (some shells may skip earlier definition when ONLY_CASE is set)
if ! declare -F case_arr_growth_pct_schema >/dev/null 2>&1; then
case_arr_growth_pct_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": [
          "date",
          "pct_7",
          "pct_30",
          "pct_90"
        ],
        "properties": {
          "date": {
            "type": "string"
          },
          "pct_7": {
            "type": [
              "number",
              "null"
            ]
          },
          "pct_30": {
            "type": [
              "number",
              "null"
            ]
          },
          "pct_90": {
            "type": [
              "number",
              "null"
            ]
          }
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}
fi

run_case_arr_discounted_growth_pct() {
  echo -e "${GREEN}=== CASE: Discounted ARR Growth % (7/30/90) ===${NC}"
  local case_name="arr_discounted_growth_pct"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_arr_growth_pct_schema >"${schema_file}"

  # Inline SQL (avoids relying on function inheritance in subshells)
  local sql_ref
  read -r -d '' sql_ref <<'SQL' || true
WITH series AS (
  SELECT
    run_date AS date,
    (
      MAX(IF(metric_name = 'arr_business_discount', metric_value, NULL))
      + COALESCE(MAX(IF(metric_name = 'arr_homelab_discount', metric_value, NULL)), 0)
      + COALESCE(MAX(IF(metric_name = 'ai_credits_space_revenue', metric_value, NULL)), 0)
      + COALESCE(MAX(IF(metric_name = 'onprem_arr', metric_value, NULL)), 0)
    ) AS metric
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
    AND run_date > DATE '2023-11-28'
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
)
SELECT
  date,
  SAFE_DIVIDE(metric - LAG(metric, 7) OVER (ORDER BY date), LAG(metric, 7) OVER (ORDER BY date)) * 100 AS pct_7,
  SAFE_DIVIDE(metric - LAG(metric, 30) OVER (ORDER BY date), LAG(metric, 30) OVER (ORDER BY date)) * 100 AS pct_30,
  SAFE_DIVIDE(metric - LAG(metric, 90) OVER (ORDER BY date), LAG(metric, 90) OVER (ORDER BY date)) * 100 AS pct_90
FROM series
WHERE date < CURRENT_DATE()
ORDER BY date;
SQL

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "${sql_ref}"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Show discounted ARR % growth over 7/30/90 days, per day, between ${FROM_DATE} and ${TO_DATE}."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_timeseries_fields "${ref_file}" "${agent_file}" '["pct_7","pct_30","pct_90"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok; ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name}"
    exit 1
  fi
}

run_case_nodes_combined_growth_pct() {
  echo -e "${GREEN}=== CASE: Business paid nodes + Homelab reachable nodes growth % ===${NC}"
  local case_name="nodes_combined_growth_pct"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_arr_growth_pct_schema >"${schema_file}"

  local metric_expr="COALESCE(MAX(IF(metric_name = 'paid_nodes_business_annual', metric_value, NULL)),0) + COALESCE(MAX(IF(metric_name = 'paid_nodes_business_monthly', metric_value, NULL)),0) + COALESCE(MAX(IF(metric_name = 'total_reachable_nodes_homelab', metric_value, NULL)),0)"
  local sql_ref
  read -r -d '' sql_ref <<'SQL' || true
WITH series AS (
  SELECT
    run_date AS date,
    __METRIC_EXPR__ AS metric
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
    AND run_date > DATE '2023-11-28'
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
)
SELECT
  date,
  SAFE_DIVIDE(metric - LAG(metric, 7) OVER (ORDER BY date), LAG(metric, 7) OVER (ORDER BY date)) * 100 AS pct_7,
  SAFE_DIVIDE(metric - LAG(metric, 30) OVER (ORDER BY date), LAG(metric, 30) OVER (ORDER BY date)) * 100 AS pct_30,
  SAFE_DIVIDE(metric - LAG(metric, 90) OVER (ORDER BY date), LAG(metric, 90) OVER (ORDER BY date)) * 100 AS pct_90
FROM series
WHERE date < CURRENT_DATE()
ORDER BY date;
SQL
  sql_ref="${sql_ref//__METRIC_EXPR__/${metric_expr}}"

  run_bq "${ref_file}" "${sql_ref}"
  run jq -e . "${ref_file}" >/dev/null

  run_agent "${agent_file}" "${schema_file}" "Show % growth over 7/30/90 days for business paid nodes plus homelab reachable nodes, per day, between ${FROM_DATE} and ${TO_DATE}."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_timeseries_fields "${ref_file}" "${agent_file}" '["pct_7","pct_30","pct_90"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok; ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name}"
    exit 1
  fi
}

run_case_nodes_reachable_growth_pct() {
  echo -e "${GREEN}=== CASE: Reachable nodes growth % ===${NC}"
  local case_name="nodes_reachable_growth_pct"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_arr_growth_pct_schema >"${schema_file}"
  local metric_expr="MAX(IF(metric_name = 'nodes_reachable', metric_value, NULL))"
  local sql_ref
  read -r -d '' sql_ref <<'SQL' || true
WITH series AS (
  SELECT
    run_date AS date,
    __METRIC_EXPR__ AS metric
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
    AND run_date > DATE '2023-11-28'
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
)
SELECT
  date,
  SAFE_DIVIDE(metric - LAG(metric, 7) OVER (ORDER BY date), LAG(metric, 7) OVER (ORDER BY date)) * 100 AS pct_7,
  SAFE_DIVIDE(metric - LAG(metric, 30) OVER (ORDER BY date), LAG(metric, 30) OVER (ORDER BY date)) * 100 AS pct_30,
  SAFE_DIVIDE(metric - LAG(metric, 90) OVER (ORDER BY date), LAG(metric, 90) OVER (ORDER BY date)) * 100 AS pct_90
FROM series
WHERE date < CURRENT_DATE()
ORDER BY date;
SQL
  sql_ref="${sql_ref//__METRIC_EXPR__/${metric_expr}}"

  run_bq "${ref_file}" "${sql_ref}"
  run jq -e . "${ref_file}" >/dev/null

  run_agent "${agent_file}" "${schema_file}" "Show % growth over 7/30/90 days for reachable nodes, per day, between ${FROM_DATE} and ${TO_DATE}."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_timeseries_fields "${ref_file}" "${agent_file}" '["pct_7","pct_30","pct_90"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok; ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name}"
    exit 1
  fi
}

run_case_business_nodes_growth_pct() {
  echo -e "${GREEN}=== CASE: Business paid nodes growth % ===${NC}"
  local case_name="business_nodes_growth_pct"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_arr_growth_pct_schema >"${schema_file}"
  local metric_expr="COALESCE(MAX(IF(metric_name = 'paid_nodes_business_annual', metric_value, NULL)),0) + COALESCE(MAX(IF(metric_name = 'paid_nodes_business_monthly', metric_value, NULL)),0)"
  local sql_ref
  read -r -d '' sql_ref <<'SQL' || true
WITH series AS (
  SELECT
    run_date AS date,
    __METRIC_EXPR__ AS metric
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
    AND run_date > DATE '2023-11-28'
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
)
SELECT
  date,
  SAFE_DIVIDE(metric - LAG(metric, 7) OVER (ORDER BY date), LAG(metric, 7) OVER (ORDER BY date)) * 100 AS pct_7,
  SAFE_DIVIDE(metric - LAG(metric, 30) OVER (ORDER BY date), LAG(metric, 30) OVER (ORDER BY date)) * 100 AS pct_30,
  SAFE_DIVIDE(metric - LAG(metric, 90) OVER (ORDER BY date), LAG(metric, 90) OVER (ORDER BY date)) * 100 AS pct_90
FROM series
WHERE date < CURRENT_DATE()
ORDER BY date;
SQL
  sql_ref="${sql_ref//__METRIC_EXPR__/${metric_expr}}"

  run_bq "${ref_file}" "${sql_ref}"
  run jq -e . "${ref_file}" >/dev/null

  run_agent "${agent_file}" "${schema_file}" "Show % growth over 7/30/90 days for business paid nodes, per day, between ${FROM_DATE} and ${TO_DATE}."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_timeseries_fields "${ref_file}" "${agent_file}" '["pct_7","pct_30","pct_90"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok; ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name}"
    exit 1
  fi
}

run_case_customers_growth_pct() {
  echo -e "${GREEN}=== CASE: Customers (subscriptions) growth % ===${NC}"
  local case_name="customers_growth_pct"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_arr_growth_pct_schema >"${schema_file}"
  local metric_expr="MAX(IF(metric_name = 'total_business_subscriptions', metric_value, NULL)) + COALESCE(MAX(IF(metric_name = 'total_homelab_subscriptions', metric_value, NULL)), 0) + 0"
  local sql_ref
  read -r -d '' sql_ref <<'SQL' || true
WITH series AS (
  SELECT
    run_date AS date,
    __METRIC_EXPR__ AS metric
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
    AND run_date > DATE '2023-11-28'
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
)
SELECT
  date,
  SAFE_DIVIDE(metric - LAG(metric, 7) OVER (ORDER BY date), LAG(metric, 7) OVER (ORDER BY date)) * 100 AS pct_7,
  SAFE_DIVIDE(metric - LAG(metric, 30) OVER (ORDER BY date), LAG(metric, 30) OVER (ORDER BY date)) * 100 AS pct_30,
  SAFE_DIVIDE(metric - LAG(metric, 90) OVER (ORDER BY date), LAG(metric, 90) OVER (ORDER BY date)) * 100 AS pct_90
FROM series
WHERE date < CURRENT_DATE()
ORDER BY date;
SQL
  sql_ref="${sql_ref//__METRIC_EXPR__/${metric_expr}}"

  run_bq "${ref_file}" "${sql_ref}"
  run jq -e . "${ref_file}" >/dev/null

  run_agent "${agent_file}" "${schema_file}" "Show % growth over 7/30/90 days for customers/subscriptions (business + homelab), per day, between ${FROM_DATE} and ${TO_DATE}."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_timeseries_fields "${ref_file}" "${agent_file}" '["pct_7","pct_30","pct_90"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok; ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name}"
    exit 1
  fi
}

run_case_homelab_nodes_growth_pct() {
  echo -e "${GREEN}=== CASE: Homelab reachable nodes growth % ===${NC}"
  local case_name="homelab_nodes_growth_pct"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_arr_growth_pct_schema >"${schema_file}"
  local metric_expr="MAX(IF(metric_name = 'total_reachable_nodes_homelab', metric_value, NULL))"
  local sql_ref
  read -r -d '' sql_ref <<'SQL' || true
WITH series AS (
  SELECT
    run_date AS date,
    __METRIC_EXPR__ AS metric
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
    AND run_date > DATE '2023-11-28'
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
)
SELECT
  date,
  SAFE_DIVIDE(metric - LAG(metric, 7) OVER (ORDER BY date), LAG(metric, 7) OVER (ORDER BY date)) * 100 AS pct_7,
  SAFE_DIVIDE(metric - LAG(metric, 30) OVER (ORDER BY date), LAG(metric, 30) OVER (ORDER BY date)) * 100 AS pct_30,
  SAFE_DIVIDE(metric - LAG(metric, 90) OVER (ORDER BY date), LAG(metric, 90) OVER (ORDER BY date)) * 100 AS pct_90
FROM series
WHERE date < CURRENT_DATE()
ORDER BY date;
SQL
  sql_ref="${sql_ref//__METRIC_EXPR__/${metric_expr}}"

  run_bq "${ref_file}" "${sql_ref}"
  run jq -e . "${ref_file}" >/dev/null

  run_agent "${agent_file}" "${schema_file}" "Show % growth over 7/30/90 days for homelab reachable nodes, per day, between ${FROM_DATE} and ${TO_DATE}."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_timeseries_fields "${ref_file}" "${agent_file}" '["pct_7","pct_30","pct_90"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok; ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name}"
    exit 1
  fi
}

run_case_arr_undiscounted_growth_pct() {
  echo -e "${GREEN}=== CASE: Undiscounted ARR Growth % (7/30/90) ===${NC}"
  local case_name="arr_undiscounted_growth_pct"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_arr_growth_pct_schema >"${schema_file}"

  local sql_ref
  read -r -d '' sql_ref <<'SQL' || true
WITH series AS (
  SELECT
    run_date AS date,
    (
      MAX(IF(metric_name = 'arr_business', metric_value, NULL))
      + COALESCE(MAX(IF(metric_name = 'arr_homelab', metric_value, NULL)), 0)
      + COALESCE(MAX(IF(metric_name = 'ai_credits_space_revenue', metric_value, NULL)), 0)
      + COALESCE(MAX(IF(metric_name = 'onprem_arr', metric_value, NULL)), 0)
    ) AS metric
  FROM `netdata-analytics-bi.metrics.metrics_daily`
  WHERE run_date BETWEEN @from_date AND @to_date
    AND run_date > DATE '2023-11-28'
    AND run_date < CURRENT_DATE()
  GROUP BY run_date
)
SELECT
  date,
  SAFE_DIVIDE(metric - LAG(metric, 7) OVER (ORDER BY date), LAG(metric, 7) OVER (ORDER BY date)) * 100 AS pct_7,
  SAFE_DIVIDE(metric - LAG(metric, 30) OVER (ORDER BY date), LAG(metric, 30) OVER (ORDER BY date)) * 100 AS pct_30,
  SAFE_DIVIDE(metric - LAG(metric, 90) OVER (ORDER BY date), LAG(metric, 90) OVER (ORDER BY date)) * 100 AS pct_90
FROM series
WHERE date < CURRENT_DATE()
ORDER BY date;
SQL

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "${sql_ref}"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Show undiscounted ARR % growth over 7/30/90 days, per day, between ${FROM_DATE} and ${TO_DATE}."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_timeseries_fields "${ref_file}" "${agent_file}" '["pct_7","pct_30","pct_90"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok; ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name}"
    exit 1
  fi
}

run_case_business_plan_services_money() {
  echo -e "${GREEN}=== CASE: Business plan services money (stat lastNotNull) ===${NC}"

  local case_name="business_plan_services_money"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_business_plan_services_money_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_business_plan_services_money_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Give me the latest business_plan_services_money value within ${FROM_DATE}..${TO_DATE}."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields "${ref_file}" "${agent_file}" '["business_plan_services_money"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_total_arr_plus_unrealized_arr() {
  echo -e "${GREEN}=== CASE: Total ARR + Unrealized ARR (stat lastNotNull) ===${NC}"

  local case_name="total_arr_plus_unrealized_arr"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_total_arr_plus_unrealized_arr_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_total_arr_plus_unrealized_arr_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Provide the latest total ARR (realized discounted + unrealized newcomer ARR) within ${FROM_DATE}..${TO_DATE}."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields "${ref_file}" "${agent_file}" '["total_arr_discounted"]' 0.01 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_unrealized_arr() {
  echo -e "${GREEN}=== CASE: Unrealized ARR (stat lastNotNull) ===${NC}"

  local case_name="unrealized_arr"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_unrealized_arr_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_unrealized_arr_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Give me the latest unrealized ARR (business and homelab newcomer ARR) within ${FROM_DATE}..${TO_DATE}."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields "${ref_file}" "${agent_file}" '["business","homelab"]' 0.01 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_unrealized_arr_barchart_snapshot() {
  echo -e "${GREEN}=== CASE: Unrealized ARR barchart (snapshot; to-be-realized by realized date) ===${NC}"

  local case_name="unrealized_arr_barchart_snapshot"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_unrealized_arr_barchart_snapshot_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (snapshot as-of ${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_unrealized_arr_barchart_snapshot_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Show unrealized ARR amounts grouped by expected realization date."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_timeseries_fields "${ref_file}" "${agent_file}" '["to_be_realized"]' 0.01 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_realized_arr_stat() {
  echo -e "${GREEN}=== CASE: $ Realized ARR (stat lastNotNull) ===${NC}"

  local case_name="realized_arr_stat"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_realized_arr_stat_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_realized_arr_stat_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" ""
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields "${ref_file}" "${agent_file}" '["realized_arr"]' 0.01 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_realized_arr_deltas() {
  echo -e "${GREEN}=== CASE: Realized ARR deltas (stat 7/30/90) ===${NC}"

  local case_name="realized_arr_deltas"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_realized_arr_deltas_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (as-of ${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_realized_arr_deltas_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Calculate realized ARR deltas as of ${TO_DATE}: last_7_days, last_30_days, last_90_days."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields "${ref_file}" "${agent_file}" '["last_7_days","last_30_days","last_90_days"]' 0.01 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_ending_trial_spaces_barchart_snapshot() {
  echo -e "${GREEN}=== CASE: Ending Trial Spaces (snapshot barchart; reachable nodes sum by end date) ===${NC}"

  local case_name="ending_trial_spaces_barchart_snapshot"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_ending_trial_spaces_barchart_snapshot_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (snapshot as-of ${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_ending_trial_spaces_barchart_snapshot_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_timeseries_fields "${ref_file}" "${agent_file}" '["reachable_nodes_sum"]' 0.01 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_new_business_subscriptions() {
  echo -e "${GREEN}=== CASE: New Business Subscriptions (timeseries) ===${NC}"

  local case_name="new_business_subscriptions"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_new_business_subscriptions_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_new_business_subscriptions_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Show daily new business subscriptions between ${FROM_DATE} and ${TO_DATE}."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_timeseries_fields "${ref_file}" "${agent_file}" '["new_business_subs"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_churned_business_subscriptions() {
  echo -e "${GREEN}=== CASE: Churned Business Subscriptions (timeseries) ===${NC}"

  local case_name="churned_business_subscriptions"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_churned_business_subscriptions_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_churned_business_subscriptions_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Show daily churned business subscriptions between ${FROM_DATE} and ${TO_DATE}."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_timeseries_fields "${ref_file}" "${agent_file}" '["churn_business_subs"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_ai_bundle_metrics() {
  echo -e "${GREEN}=== CASE: AI Bundle metrics (timeseries) ===${NC}"

  local case_name="ai_bundle_metrics"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_ai_bundle_metrics_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_ai_bundle_metrics_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Show daily AI bundle metrics between ${FROM_DATE} and ${TO_DATE}: bundle_625, bundle_300, bundle_1100, bundle_2000."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_timeseries_fields "${ref_file}" "${agent_file}" '["bundle_625","bundle_300","bundle_1100","bundle_2000"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_ai_credits_spaces() {
  echo -e "${GREEN}=== CASE: Spaces with AI Credits (stat lastNotNull) ===${NC}"

  local case_name="ai_credits_spaces"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_ai_credits_space_count_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_ai_credits_space_count_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Give me the latest count of spaces with AI credits within ${FROM_DATE}..${TO_DATE}."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields "${ref_file}" "${agent_file}" '["spaces"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_business_subscriptions() {
  echo -e "${GREEN}=== CASE: Business Subscriptions (stat lastNotNull) ===${NC}"

  local case_name="business_subscriptions"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_business_subscriptions_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_business_subscriptions_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Give me the latest business subscriptions count within ${FROM_DATE}..${TO_DATE}."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields "${ref_file}" "${agent_file}" '["cloud_subscriptions"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_business_arr_discount() {
  echo -e "${GREEN}=== CASE: Business ARR (stat last/lastNotNull) ===${NC}"

  local case_name="business_arr_discount"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_business_arr_discount_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_business_arr_discount_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Give me the latest business ARR (discounted) within ${FROM_DATE}..${TO_DATE}."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields "${ref_file}" "${agent_file}" '["discounted_arr"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_business_nodes() {
  echo -e "${GREEN}=== CASE: Business Nodes (stat last/lastNotNull) ===${NC}"

  local case_name="business_nodes"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_business_nodes_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_business_nodes_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Give me the latest business paid nodes (annual + monthly) within ${FROM_DATE}..${TO_DATE}."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields "${ref_file}" "${agent_file}" '["total"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_windows_reachable_nodes() {
  echo -e "${GREEN}=== CASE: Windows Reachable Nodes (stat lastNotNull) ===${NC}"

  local case_name="windows_reachable_nodes"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_windows_reachable_nodes_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_windows_reachable_nodes_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Give me the latest reachable Windows nodes within ${FROM_DATE}..${TO_DATE}."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields "${ref_file}" "${agent_file}" '["windows_nodes"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_homelab_subscriptions() {
  echo -e "${GREEN}=== CASE: Homelab Subscriptions (stat lastNotNull) ===${NC}"

  local case_name="homelab_subscriptions"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_homelab_subscriptions_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_homelab_subscriptions_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Give me the latest homelab subscriptions count within ${FROM_DATE}..${TO_DATE}."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields "${ref_file}" "${agent_file}" '["homelab_subs"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_homelab_arr_discount() {
  echo -e "${GREEN}=== CASE: Homelab ARR discounted (stat lastNotNull) ===${NC}"

  local case_name="homelab_arr_discount"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_homelab_arr_discount_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_homelab_arr_discount_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Give me the latest homelab ARR (discounted) within ${FROM_DATE}..${TO_DATE}."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields "${ref_file}" "${agent_file}" '["discounted_arr"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_homelab_nodes() {
  echo -e "${GREEN}=== CASE: Homelab nodes (stat lastNotNull) ===${NC}"

  local case_name="homelab_nodes"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_homelab_nodes_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_homelab_nodes_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Give me the latest homelab reachable nodes within ${FROM_DATE}..${TO_DATE}."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields "${ref_file}" "${agent_file}" '["total"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_on_prem_customers() {
  echo -e "${GREEN}=== CASE: On-prem customers (stat lastNotNull) ===${NC}"

  local case_name="on_prem_customers"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_on_prem_customers_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_on_prem_customers_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Give me the latest on-prem & support subscriptions within ${FROM_DATE}..${TO_DATE}; for dates <= 2025-10-04 use 5, otherwise use onprem_customers metric."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields "${ref_file}" "${agent_file}" '["customers"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_on_prem_arr() {
  echo -e "${GREEN}=== CASE: On Prem ARR (stat lastNotNull) ===${NC}"

  local case_name="on_prem_arr"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_on_prem_arr_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_on_prem_arr_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Give me the latest on-prem ARR within ${FROM_DATE}..${TO_DATE}."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields "${ref_file}" "${agent_file}" '["onprem_arr"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_windows_reachable_nodes_breakdown() {
  echo -e "${GREEN}=== CASE: Windows Reachable Nodes breakdown (timeseries) ===${NC}"

  local case_name="windows_reachable_nodes_breakdown"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_windows_reachable_nodes_breakdown_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_windows_reachable_nodes_breakdown_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Show daily reachable Windows nodes by segment (business, trial, community, homelab) between ${FROM_DATE} and ${TO_DATE}."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_timeseries_fields "${ref_file}" "${agent_file}" '["business","trial","community","homelab"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_saas_spaces_counts_snapshot() {
  echo -e "${GREEN}=== CASE: SaaS Spaces (snapshot stat; counts by segment) ===${NC}"

  local case_name="saas_spaces_counts_snapshot"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_saas_spaces_counts_snapshot_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (snapshot as-of ${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_saas_spaces_counts_snapshot_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "From the spaces snapshot of ${TO_DATE}, return counts of spaces by segment (Business, Business_<45d, Homelab, Homelab_<45d, Trials_0_nodes, Trials_1-5_nodes, Trials_6+_nodes, Community_0_nodes, Community_w_nodes, Total)."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields_no_date "${ref_file}" "${agent_file}" '["Business","Business_<45d","Homelab","Homelab_<45d","Trials_0_nodes","Trials_1-5_nodes","Trials_6+_nodes","Community_0_nodes","Community_w_nodes","Total"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_saas_spaces_percent_snapshot() {
  echo -e "${GREEN}=== CASE: SaaS Spaces Total view (snapshot pie; % by segment) ===${NC}"

  local case_name="saas_spaces_percent_snapshot"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_saas_spaces_percent_snapshot_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (snapshot as-of ${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_saas_spaces_percent_snapshot_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "From the spaces snapshot of ${TO_DATE}, return percentage share of spaces by segment (Business, Business_<45d, Homelab, Homelab_<45d, Trials_0_nodes, Trials_1-5_nodes, Trials_6+_nodes, Community_0_nodes, Community_w_nodes) and a check field."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields_no_date "${ref_file}" "${agent_file}" '["Business","Business_<45d","Homelab","Homelab_<45d","Trials_0_nodes","Trials_1-5_nodes","Trials_6+_nodes","Community_0_nodes","Community_w_nodes","check"]' 0.01 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_nodes_total_view_percent_snapshot() {
  echo -e "${GREEN}=== CASE: Nodes Total View (snapshot pie; reachable nodes % by segment) ===${NC}"

  local case_name="nodes_total_view_percent_snapshot"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_nodes_total_view_percent_snapshot_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (snapshot as-of ${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_nodes_total_view_percent_snapshot_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "From the spaces snapshot of ${TO_DATE}, return reachable nodes percentage by segment (Business, Business_<45d, Homelab, Homelab_<45d, Trials_1-5_nodes, Trials_6+_nodes, Community_w_nodes) and a check field."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields_no_date "${ref_file}" "${agent_file}" '["Business","Business_<45d","Homelab","Homelab_<45d","Trials_1-5_nodes","Trials_6+_nodes","Community_w_nodes","check"]' 0.01 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_nodes_counts_snapshot() {
  echo -e "${GREEN}=== CASE: Nodes (snapshot stat; reachable nodes totals by segment) ===${NC}"

  local case_name="nodes_counts_snapshot"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_nodes_counts_snapshot_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (snapshot as-of ${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_nodes_counts_snapshot_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "From the spaces snapshot of ${TO_DATE}, return reachable nodes totals by segment (Business, Business_<45d, Homelab, Homelab_<45d, Trials_1-5_nodes, Trials_6+_nodes, Community_w_nodes, Total)."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields_no_date "${ref_file}" "${agent_file}" '["Business","Business_<45d","Homelab","Homelab_<45d","Trials_1-5_nodes","Trials_6+_nodes","Community_w_nodes","Total"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_realized_arr_kpi_stat() {
  echo -e "${GREEN}=== CASE: KPI realized_arr stat (total + per product) ===${NC}"

  local case_name="realized_arr_kpi_stat"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_realized_arr_kpi_stat_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_realized_arr_kpi_stat_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" ""
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields "${ref_file}" "${agent_file}" '["business","homelab","ai_bundles","on_prem","total"]' 0.01 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_realized_arr_kpi_timeseries() {
  echo -e "${GREEN}=== CASE: KPI realized_arr timeseries (per product + total) ===${NC}"

  local case_name="realized_arr_kpi_timeseries"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_realized_arr_kpi_timeseries_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_realized_arr_kpi_timeseries_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "JSON data[{date,business,homelab,ai_bundles,on_prem,total}], notes[]."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_timeseries_fields "${ref_file}" "${agent_file}" '["business","homelab","ai_bundles","on_prem","total"]' 0.01 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_realized_arr_kpi_delta() {
  echo -e "${GREEN}=== CASE: KPI realized_arr delta (per product + total) ===${NC}"

  local case_name="realized_arr_kpi_delta"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_realized_arr_kpi_delta_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_realized_arr_kpi_delta_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" ""
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields "${ref_file}" "${agent_file}" '["business_delta","homelab_delta","ai_bundles_delta","on_prem_delta","total_delta"]' 0.01 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_realized_arr_kpi_customer_diff() {
  echo -e "${GREEN}=== CASE: KPI realized_arr customer diff (won/lost/increase/decrease) ===${NC}"

  local case_name="realized_arr_kpi_customer_diff"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_realized_arr_kpi_customer_diff_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (snapshots ${FROM_DATE} -> ${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_realized_arr_kpi_customer_diff_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Show the top 10 customers with ARR gains (won or increased) and top 10 with ARR losses (lost or decreased) between ${FROM_DATE} and ${TO_DATE} (arr_t0 vs arr_t1). Sort gains by descending delta, losses by ascending delta."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_customer_delta_fields "${ref_file}" "${agent_file}" 0.01 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_realized_arr_kpi_customer_diff_since_only() {
  echo -e "${GREEN}=== CASE: KPI realized_arr customer diff (since <date>, implicit to=yesterday) ===${NC}"

  local case_name="realized_arr_kpi_customer_diff_since_only"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_realized_arr_kpi_customer_diff_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_realized_arr_kpi_customer_diff_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Since ${FROM_DATE}, give me the top 10 customers with ARR gains (won or increased) and top 10 with ARR losses (lost or decreased); use yesterday as the end date if I did not provide one."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_customer_delta_fields "${ref_file}" "${agent_file}" 0.01 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_business_nodes_delta_top10() {
  echo -e "${GREEN}=== CASE: Business nodes delta top 10 (increase/decrease) ===${NC}"

  local case_name="business_nodes_delta_top10"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_space_nodes_delta_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (snapshots ${FROM_DATE} -> ${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_business_nodes_delta_top10_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Show top 10 business spaces adding reachable nodes and top 10 removing reachable nodes between ${FROM_DATE} and ${TO_DATE}. Use business spaces only, no trials."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_space_nodes_delta_fields "${ref_file}" "${agent_file}" 0.01 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_homelab_nodes_delta_top10() {
  echo -e "${GREEN}=== CASE: Homelab nodes delta top 10 (increase/decrease) ===${NC}"

  local case_name="homelab_nodes_delta_top10"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_space_nodes_delta_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (snapshots ${FROM_DATE} -> ${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_homelab_nodes_delta_top10_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Show top 10 homelab spaces adding reachable nodes and top 10 removing reachable nodes between ${FROM_DATE} and ${TO_DATE}. Use homelab spaces only, no trials."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_space_nodes_delta_fields "${ref_file}" "${agent_file}" 0.01 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_business_subscriptions_deltas() {
  echo -e "${GREEN}=== CASE: Business Subscriptions deltas (stat 7/30/90) ===${NC}"

  local case_name="business_subscriptions_deltas"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_business_subscriptions_deltas_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (as-of ${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_business_subscriptions_deltas_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Compute business subscriptions deltas as of ${TO_DATE}: last_7_days, last_30_days, last_90_days."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields "${ref_file}" "${agent_file}" '["last_7_days","last_30_days","last_90_days"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_business_arr_discount_deltas() {
  echo -e "${GREEN}=== CASE: Business ARR discounted deltas (stat 7/30/90) ===${NC}"

  local case_name="business_arr_discount_deltas"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_business_arr_discount_deltas_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (as-of ${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_business_arr_discount_deltas_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Compute business ARR (discounted) deltas as of ${TO_DATE}: last_7_days, last_30_days, last_90_days."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields "${ref_file}" "${agent_file}" '["last_7_days","last_30_days","last_90_days"]' 0.01 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

run_case_business_nodes_deltas() {
  echo -e "${GREEN}=== CASE: Business Nodes deltas (stat 7/30/90) ===${NC}"

  local case_name="business_nodes_deltas"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_business_nodes_deltas_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (as-of ${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_business_nodes_deltas_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Compute business nodes deltas (paid_nodes_business_annual + paid_nodes_business_monthly) as of ${TO_DATE}: last_7_days, last_30_days, last_90_days."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields "${ref_file}" "${agent_file}" '["last_7_days","last_30_days","last_90_days"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok
  ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

# --- AWS ARR (stat) ---
case_aws_arr_sql() {
  cat <<'SQL'
SELECT
  run_date AS date,
  MAX(IF(metric_name = 'aws_business_arr', metric_value, NULL)) AS total,
  MAX(IF(metric_name = 'aws_annual_business_arr', metric_value, NULL)) AS annual,
  MAX(IF(metric_name = 'aws_monthly_business_arr', metric_value, NULL)) AS monthly
FROM `netdata-analytics-bi.metrics.metrics_daily`
WHERE run_date BETWEEN @from_date AND @to_date
  AND run_date > DATE '2023-11-28'
  AND run_date < CURRENT_DATE()
GROUP BY run_date
HAVING total IS NOT NULL
ORDER BY date DESC
LIMIT 1;
SQL
}

case_aws_arr_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "date",
        "total",
        "annual",
        "monthly"
      ],
      "properties": {
        "date": {
          "type": "string"
        },
        "total": {
          "type": "number"
        },
        "annual": {
          "type": "number"
        },
        "monthly": {
          "type": "number"
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

run_case_aws_arr() {
  echo -e "${GREEN}=== CASE: AWS ARR (stat lastNotNull) ===${NC}"

  local case_name="aws_arr"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_aws_arr_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_aws_arr_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Give me the latest AWS ARR (total, annual, monthly) within ${FROM_DATE}..${TO_DATE}."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields "${ref_file}" "${agent_file}" '["total","annual","monthly"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok; ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

# --- AWS subscriptions (stat) ---
case_aws_subscriptions_sql() {
  cat <<'SQL'
SELECT
  run_date AS date,
  MAX(IF(metric_name = 'aws_business_subscriptions', metric_value, NULL)) AS total,
  MAX(IF(metric_name = 'aws_annual_business_subscriptions', metric_value, NULL)) AS annual,
  MAX(IF(metric_name = 'aws_monthly_business_subscriptions', metric_value, NULL)) AS monthly
FROM `netdata-analytics-bi.metrics.metrics_daily`
WHERE run_date BETWEEN @from_date AND @to_date
  AND run_date > DATE '2023-11-28'
  AND run_date < CURRENT_DATE()
GROUP BY run_date
HAVING total IS NOT NULL
ORDER BY date DESC
LIMIT 1;
SQL
}

case_aws_subscriptions_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "date",
        "total",
        "annual",
        "monthly"
      ],
      "properties": {
        "date": {
          "type": "string"
        },
        "total": {
          "type": "number"
        },
        "annual": {
          "type": "number"
        },
        "monthly": {
          "type": "number"
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

run_case_aws_subscriptions() {
  echo -e "${GREEN}=== CASE: AWS subscriptions (stat lastNotNull) ===${NC}"

  local case_name="aws_subscriptions"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_aws_subscriptions_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_aws_subscriptions_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Give me the latest AWS business subscriptions (total, annual, monthly) within ${FROM_DATE}..${TO_DATE}."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields "${ref_file}" "${agent_file}" '["total","annual","monthly"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok; ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

# --- Virtual nodes (stat) ---
case_virtual_nodes_sql() {
  cat <<'SQL'
SELECT
  run_date AS date,
  MAX(IF(metric_name = 'total_virtual_nodes_count', metric_value, NULL)) AS total_virtual_nodes_count
FROM `netdata-analytics-bi.metrics.metrics_daily`
WHERE run_date BETWEEN @from_date AND @to_date
  AND run_date > DATE '2023-11-28'
  AND run_date < CURRENT_DATE()
GROUP BY run_date
HAVING total_virtual_nodes_count IS NOT NULL
ORDER BY date DESC
LIMIT 1;
SQL
}

case_virtual_nodes_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "date",
        "total_virtual_nodes_count"
      ],
      "properties": {
        "date": {
          "type": "string"
        },
        "total_virtual_nodes_count": {
          "type": "number"
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

run_case_virtual_nodes() {
  echo -e "${GREEN}=== CASE: Virtual node count (stat lastNotNull) ===${NC}"

  local case_name="virtual_nodes"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_virtual_nodes_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_virtual_nodes_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Give me the latest virtual node count within ${FROM_DATE}..${TO_DATE}."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields "${ref_file}" "${agent_file}" '["total_virtual_nodes_count"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok; ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

# --- Active users (stat) ---
case_active_users_sql() {
  cat <<'SQL'
SELECT
  run_date AS date,
  MAX(IF(metric_name = 'spaces_active_members_sum', metric_value, NULL)) AS active_members
FROM `netdata-analytics-bi.metrics.metrics_daily`
WHERE run_date BETWEEN @from_date AND @to_date
  AND run_date > DATE '2023-11-28'
  AND run_date < CURRENT_DATE()
GROUP BY run_date
HAVING active_members IS NOT NULL
ORDER BY date DESC
LIMIT 1;
SQL
}

case_top_customers_arr_2k_sql() {
  cat <<'SQL'
WITH sub_latest AS (
  SELECT
    space_id,
    ARRAY_AGG(STRUCT(
      DATE(SAFE_CAST(created_at AS TIMESTAMP)) AS created_date,
      LOWER(NULLIF(period, '')) AS period,
      SAFE_CAST(committed_nodes AS INT64) AS committed_nodes
    ) ORDER BY SAFE_CAST(created_at AS TIMESTAMP) DESC LIMIT 1)[OFFSET(0)] AS sub
  FROM `netdata-analytics-bi.app_db_replication.spaceroom_space_active_subscriptions_latest`
  GROUP BY space_id
),
admins AS (
  SELECT
    space_id,
    STRING_AGG(contact, ', ' ORDER BY contact) AS admin_contacts
  FROM (
    SELECT DISTINCT
      m.space_id,
      CASE
        WHEN a.name IS NOT NULL AND a.name != '' THEN CONCAT(a.name, ' <', a.email, '>')
        ELSE a.email
      END AS contact
    FROM `netdata-analytics-bi.app_db_replication.spaceroom_space_members_latest` m
    JOIN `netdata-analytics-bi.app_db_replication.account_accounts_latest` a
      ON a.id = m.account_id
    WHERE m.role = 'admin'
  )
  GROUP BY space_id
),
base AS (
  SELECT
    s.aa_space_id AS space_id,
    s.ab_space_name AS space_name,
    s.ce_plan_class AS plan_class,
    COALESCE(s.bq_arr_discount, 0) AS current_arr,
    s.ca_cur_plan_start_date AS start_date,
    LOWER(NULLIF(s.bc_period, '')) AS wt_period,
    s.ae_reachable_nodes AS connected_nodes,
    sub_latest.sub.period AS sub_period,
    sub_latest.sub.committed_nodes AS committed_nodes,
    COALESCE(admins.admin_contacts, 'unknown') AS primary_contact
  FROM `netdata-analytics-bi.watch_towers.spaces_latest` s
  LEFT JOIN sub_latest
    ON sub_latest.space_id = s.aa_space_id
  LEFT JOIN admins
    ON admins.space_id = s.aa_space_id
  WHERE s.ce_plan_class IN (
    'Business',
    'Homelab',
    'Business_45d_monthly_newcomer',
    'Homelab_45d_monthly_newcomer'
  )
    AND (s.ax_trial_ends_at IS NULL OR s.ax_trial_ends_at = '')
)
SELECT
  space_id,
  space_name,
  plan_class,
  primary_contact,
  CASE
    WHEN start_date IS NULL THEN 'unknown'
    WHEN COALESCE(sub_period, wt_period) = 'year'
      THEN FORMAT_DATE('%Y-%m-%d', DATE_ADD(start_date, INTERVAL 1 YEAR))
    WHEN COALESCE(sub_period, wt_period) = 'month'
      THEN FORMAT_DATE('%Y-%m-%d', DATE_ADD(start_date, INTERVAL 1 MONTH))
    ELSE 'unknown'
  END AS renewal_date,
  current_arr,
  committed_nodes,
  connected_nodes,
  current_arr AS renewal_forecast_arr
FROM base
WHERE current_arr >= 2000
ORDER BY current_arr DESC, space_id
LIMIT 100;
SQL
}

case_top_customers_arr_2k_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "array",
      "minItems": 1,
      "maxItems": 100,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": [
          "space_id",
          "space_name",
          "plan_class",
          "primary_contact",
          "renewal_date",
          "current_arr",
          "committed_nodes",
          "connected_nodes",
          "renewal_forecast_arr"
        ],
        "properties": {
          "space_id": {
            "type": "string"
          },
          "space_name": {
            "type": "string"
          },
          "plan_class": {
            "type": "string"
          },
          "primary_contact": {
            "type": "string"
          },
          "renewal_date": {
            "type": "string"
          },
          "current_arr": {
            "type": "number"
          },
          "committed_nodes": {
            "type": [
              "number",
              "null"
            ]
          },
          "connected_nodes": {
            "type": "number"
          },
          "renewal_forecast_arr": {
            "type": "number"
          }
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

run_case_top_customers_arr_2k() {
  echo -e "${GREEN}=== CASE: Top 100 customers >= \$2K ARR (current) ===${NC}"

  local case_name="top_customers_arr_2k"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_top_customers_arr_2k_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (current) --${NC}"
  run_bq "${ref_file}" "$(case_top_customers_arr_2k_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" $'\\n\\nCan you create a list of the top 100 customers paying $2K or more in ARR for Netdata subscriptions and identifying the following:\\n- Primary Contact\\n- Subscription Renewal Date\\n- Current ARR\\n- Number of Nodes committed\\n- Number of Nodes connected\\nFor annual subscriptions, use 1 year after the subscription start date as the renewal date and the renewal forecast ARR should be same as the current ARR. The space owner or the admins in the space can be marked as primary contacts for the customer.\\n\\n'
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_keyed_rows_fields "${ref_file}" "${agent_file}" "space_id" '["current_arr","committed_nodes","connected_nodes","renewal_forecast_arr"]' '["space_name","plan_class","primary_contact","renewal_date"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok; ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

case_active_users_schema() {
  cat <<'JSON'
{
  "type": "object",
  "additionalProperties": false,
  "required": [
    "data",
    "notes",
    "data_freshness"
  ],
  "properties": {
    "data": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "date",
        "active_members"
      ],
      "properties": {
        "date": {
          "type": "string"
        },
        "active_members": {
          "type": "number"
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "data_freshness": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "last_ingested_at",
        "age_minutes",
        "source_table"
      ],
      "properties": {
        "last_ingested_at": {
          "type": "string"
        },
        "age_minutes": {
          "type": "number"
        },
        "source_table": {
          "type": "string"
        }
      }
    }
  }
}
JSON
}

run_case_active_users() {
  echo -e "${GREEN}=== CASE: Active users (stat lastNotNull) ===${NC}"

  local case_name="active_users"
  local ref_file="${OUT_DIR}/${case_name}.ref.json"
  local agent_file="${OUT_DIR}/${case_name}.agent.json"
  local schema_file="${OUT_DIR}/${case_name}.schema.json"
  local diff_file="${OUT_DIR}/${case_name}.diff.json"

  case_active_users_schema >"${schema_file}"

  echo -e "${YELLOW}-- BigQuery reference (${FROM_DATE}..${TO_DATE}) --${NC}"
  run_bq "${ref_file}" "$(case_active_users_sql)"
  run jq -e . "${ref_file}" >/dev/null

  echo -e "${YELLOW}-- Agent response (schema-enforced JSON) --${NC}"
  run_agent "${agent_file}" "${schema_file}" "Give me the latest active users (spaces_active_members_sum) within ${FROM_DATE}..${TO_DATE}."
  run jq -e 'if (.data | type) == "array" then (.data | length >= 1) elif (.data | type) == "object" then true else false end' "${agent_file}" >/dev/null

  echo -e "${YELLOW}-- Compare --${NC}"
  compare_single_row_fields "${ref_file}" "${agent_file}" '["active_members"]' 0.0001 | tee "${diff_file}" >/dev/null
  local ok; ok="$(jq -r '.ok' "${diff_file}")"
  if [[ "${ok}" == "true" ]]; then
    echo -e "${GREEN}[PASS]${NC} ${case_name}"
  else
    echo -e "${RED}[FAIL]${NC} ${case_name} (see ${diff_file})" >&2
    run jq . "${diff_file}"
    exit 1
  fi
}

ALL_CASES=(
  realized_arr
  realized_arr_components
  realized_arr_percent
  trials_total
  trial_6plus_nodes_est_value
  business_plan_services_money
  total_arr_plus_unrealized_arr
  unrealized_arr
  unrealized_arr_barchart_snapshot
  realized_arr_stat
  realized_arr_deltas
  ending_trial_spaces_barchart_snapshot
  new_business_subscriptions
  churned_business_subscriptions
  ai_bundle_metrics
  ai_credits_spaces
  business_subscriptions
  business_arr_discount
  business_nodes
  windows_reachable_nodes
  saas_spaces_counts_snapshot
  saas_spaces_percent_snapshot
  nodes_total_view_percent_snapshot
  business_subscriptions_deltas
  business_arr_discount_deltas
  business_nodes_deltas
  arr_discounted_growth_pct
  arr_undiscounted_growth_pct
  nodes_combined_growth_pct
  nodes_reachable_growth_pct
  business_nodes_growth_pct
  customers_growth_pct
  homelab_nodes_growth_pct
  homelab_subscriptions
  homelab_arr_discount
  homelab_nodes
  on_prem_customers
  on_prem_arr
  nodes_counts_snapshot
  windows_reachable_nodes_breakdown
  realized_arr_kpi_stat
  realized_arr_kpi_timeseries
  realized_arr_kpi_delta
  realized_arr_kpi_customer_diff
  realized_arr_kpi_customer_diff_since_only
  business_nodes_delta_top10
  homelab_nodes_delta_top10
  top_customers_arr_2k
  aws_arr
  aws_subscriptions
  virtual_nodes
  active_users
)

PASSED_CASES=()
FAILED_CASES=()

run_case_wrapper() {
  local name="$1"
  local func="run_case_${name}"
  local log_file="${LOG_DIR}/${name}.log"
  echo -e "${YELLOW}-- START ${name} (log: ${log_file}) --${NC}"
  ( "${func}" ) >"${log_file}" 2>&1
}

record_result() {
  local name="$1"
  local status="$2"
  if [[ "${status}" -eq 0 ]]; then
    PASSED_CASES+=("${name}")
    echo -e "${GREEN}[PASS]${NC} ${name}"
  else
    FAILED_CASES+=("${name}")
    echo -e "${RED}[FAIL]${NC} ${name} (see ${LOG_DIR}/${name}.log)" >&2
  fi
}

run_cases_sequential() {
  local name
  for name in "${CASES_TO_RUN[@]}"; do
    if run_case_wrapper "${name}"; then
      record_result "${name}" 0
    else
      record_result "${name}" 1
      if [[ "${FAIL_FAST}" -eq 1 ]]; then
        return 1
      fi
    fi
  done
  return 0
}

run_cases_parallel() {
  local -A pid_to_case=()
  local stop_requested=0
  local name
  for name in "${CASES_TO_RUN[@]}"; do
    if [[ "${stop_requested}" -eq 1 ]]; then
      break
    fi
    while [[ "${#pid_to_case[@]}" -ge "${JOBS}" ]]; do
      local finished_pid
      wait -n -p finished_pid
      local status=$?
      local case_name="${pid_to_case[${finished_pid}]}"
      unset pid_to_case["${finished_pid}"]
      record_result "${case_name}" "${status}"
      if [[ "${status}" -ne 0 && "${FAIL_FAST}" -eq 1 ]]; then
        stop_requested=1
        break
      fi
    done
    if [[ "${stop_requested}" -eq 1 ]]; then
      break
    fi
    local log_file="${LOG_DIR}/${name}.log"
    local func="run_case_${name}"
    echo -e "${YELLOW}-- START ${name} (log: ${log_file}) --${NC}"
    ( "${func}" ) >"${log_file}" 2>&1 &
    local pid=$!
    pid_to_case["${pid}"]="${name}"
  done

  local pid
  for pid in "${!pid_to_case[@]}"; do
    wait "${pid}"
    local status=$?
    local case_name="${pid_to_case[${pid}]}"
    record_result "${case_name}" "${status}"
  done

  if [[ "${stop_requested}" -eq 1 && "${FAIL_FAST}" -eq 1 ]]; then
    return 1
  fi
  return 0
}

main() {
  local name
  CASES_TO_RUN=()
  for name in "${ALL_CASES[@]}"; do
    if should_run "${name}"; then
      CASES_TO_RUN+=("${name}")
    fi
  done

  if [[ -n "${CASE_FILTER}" && "${CASE_MATCHED}" -eq 0 ]]; then
    echo "Unknown ONLY_CASE: ${CASE_FILTER}" >&2
    return 1
  fi

  if [[ "${#CASES_TO_RUN[@]}" -eq 0 ]]; then
    echo "No cases selected." >&2
    return 1
  fi

  if [[ "${JOBS}" -le 1 ]]; then
    run_cases_sequential || true
  else
    run_cases_parallel || true
  fi

  local total="${#CASES_TO_RUN[@]}"
  local passed="${#PASSED_CASES[@]}"
  local failed="${#FAILED_CASES[@]}"
  local skipped=$(( total - passed - failed ))

  echo -e "${YELLOW}=== SUMMARY ===${NC}"
  echo "Total: ${total}, Passed: ${passed}, Failed: ${failed}, Skipped: ${skipped}"
  if [[ "${failed}" -gt 0 ]]; then
    echo "Failed cases:"
    for name in "${FAILED_CASES[@]}"; do
      echo "  - ${name} (log: ${LOG_DIR}/${name}.log)"
    done
    return 1
  fi
  return 0
}

main
