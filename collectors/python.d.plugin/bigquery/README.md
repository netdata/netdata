<!--
title: "BigQuery"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/bigquery/README.md
-->

# BigQuery Netdata Collector

A python based collector leveraging [`pandas-gbq`](https://pandas-gbq.readthedocs.io/en/latest/) to run a query against [BigQuery](https://cloud.google.com/bigquery) and save results into a chart in Netdata.

## Requirements

This collector depend on some Python packages that can usually be installed via `pip`.

```bash
sudo pip install google-auth pandas-gbq
```

## Credentials

The collector expects to read the credentials for a [service account](https://cloud.google.com/iam/docs/service-accounts) from a file defined in the collector configuration. If this is not defined then auth will fallback to the `pandas-gbq` [approach](https://pandas-gbq.readthedocs.io/en/latest/howto/authentication.html). 

## Configuration

Below is an example configuration to just query some random data from BigQuery (two times) and plot each query data as a seperate chart.

```yaml
example:
    name: "example"
    update_every: 5
    credentials: "/path/to/your/key.json"
    chart_configs:
      - chart_name: "random_data_line"
        chart_title: "some random data"
        chart_family: "bigquery"
        chart_context: "bigquery"
        chart_type: "line"
        chart_units: "n"
        project_id: "your-gcp-project-id"
        sql: "select rand()*100 as random_positive, rand()*100*-1 as random_negative"
      - chart_name: "random_data_stacked"
        chart_title: "some random data stacked"
        chart_family: "bigquery"
        chart_context: "bigquery"
        chart_type: "stacked"
        chart_units: "n"
        project_id: "your-gcp-project-id"
        sql: "select rand()*100 as random_1, rand()*100 as random_2"
```




