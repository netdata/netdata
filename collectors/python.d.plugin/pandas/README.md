<!--
title: "Pandas"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/pandas/README.md
-->

# Pandas Netdata Collector

A python collector using [pandas](https://pandas.pydata.org/) to pull data and do pandas based preprocessing before feeding to Netdata.

## Requirements

This collector depends on some Python (Python 3 only) packages that can usually be installed via `pip` or `pip3`.

```bash
$ sudo pip install pandas requests
```

## Configuration

Below is an example configuration to query some csv data from [london Netdata demo server](http://london.my-netdata.io/#after=-420;before=0;=undefined;theme=slate;utc=Europe%2FLondon), do some data wrangling on it and save in format as expected by Netdata.

```yaml
# example showing a read_csv from a url and some light pandas data wrangling.
# pull data in csv format from london demo server and then ratio of user cpus over system cpu averaged over last 60 seconds.
example_csv:
    name: "example_csv"
    update_every: 2
    chart_configs:
      - chart_name: "london_system_cpu"
        chart_title: "london_system_cpu"
        chart_family: "london_system_cpu"
        chart_context: "london_system_cpu"
        chart_type: "line"
        chart_units: "%"
        df_steps: >
          pd.read_csv('https://london.my-netdata.io/api/v1/data?chart=system.cpu&format=csv&after=-60', storage_options={'User-Agent': 'netdata'});
          df.drop('time', axis=1);
          df.mean().to_frame().transpose();
          df.apply(lambda row: (row.user / row.system), axis = 1).to_frame();
          df.rename(columns={0:'average_user_system_ratio'});
```

`chart_configs` is a list of dictionary objects where each one defines the sequence of `df_steps` to be run using `[pandas](https://pandas.pydata.org/)`, 
and the `chart_name`, `chart_title` etc to define the 
[CHART variables](https://learn.netdata.cloud/docs/agent/collectors/python.d.plugin#global-variables-order-and-chart) 
that will control how the results will look in netdata.

## Notes
  - Each line in `df_steps` must return a pandas [DataFrame](https://pandas.pydata.org/docs/reference/api/pandas.DataFrame.html) object that is called `df` at each step.
  - This collector is expecting one row in the final pandas DataFrame. It is that first row that will be taken as the most recent values for each dimension on each chart using (`df.to_dict(orient='records')[0]`). See [pd.to_dict()](https://pandas.pydata.org/docs/reference/api/pandas.DataFrame.to_dict.html).