<!--
title: "Anomaly detection with Netdata"
description: "Use ML-driven anomaly detection to narrow your focus to only affected metrics and services/processes on your node to shorten root cause analysis."
custom_edit_url: "https://github.com/netdata/netdata/edit/master/src/collectors/python.d.plugin/anomalies/README.md"
sidebar_url: "Anomalies"
sidebar_label: "anomalies"
learn_status: "Published"
learn_rel_path: "Integrations/Monitor/Anything"
-->

# Anomaly detection with Netdata

**Note**: Check out the [Netdata Anomaly Advisor](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/anomaly-advisor-tab.md) for a more native anomaly detection experience within Netdata.

This collector uses the Python [PyOD](https://pyod.readthedocs.io/en/latest/index.html) library to perform unsupervised [anomaly detection](https://en.wikipedia.org/wiki/Anomaly_detection) on your Netdata charts and/or dimensions.

Instead of this collector just _collecting_ data, it also does some computation on the data it collects to return an anomaly probability and anomaly flag for each chart or custom model you define. This computation consists of a **train** function that runs every `train_n_secs` to train the ML models to learn what 'normal' typically looks like on your node. At each iteration there is also a **predict** function that uses the latest trained models and most recent metrics to produce an anomaly probability and anomaly flag for each chart or custom model you define.

> As this is a somewhat unique collector and involves often subjective concepts like anomalies and anomaly probabilities, we would love to hear any feedback on it from the community. Please let us know on the [community forum](https://community.netdata.cloud/t/anomalies-collector-feedback-megathread/767) or drop us a note at [analytics-ml-team@netdata.cloud](mailto:analytics-ml-team@netdata.cloud) for any and all feedback, both positive and negative. This sort of feedback is priceless to help us make complex features more useful.

## Charts

Two charts are produced:

- **Anomaly Probability** (`anomalies.probability`): This chart shows the probability that the latest observed data is anomalous based on the trained model for that chart (using the [`predict_proba()`](https://pyod.readthedocs.io/en/latest/api_cc.html#pyod.models.base.BaseDetector.predict_proba) method of the trained PyOD model).
- **Anomaly** (`anomalies.anomaly`): This chart shows `1` or `0` predictions of if the latest observed data is considered anomalous or not based on the trained model (using the [`predict()`](https://pyod.readthedocs.io/en/latest/api_cc.html#pyod.models.base.BaseDetector.predict) method of the trained PyOD model).

Below is an example of the charts produced by this collector and how they might look when things are 'normal' on the node. The anomaly probabilities tend to bounce randomly around a typically low probability range, one or two might randomly jump or drift outside of this range every now and then and show up as anomalies on the anomaly chart. 

![netdata-anomalies-collector-normal](https://user-images.githubusercontent.com/2178292/100663699-99755000-334e-11eb-922f-0c41a0176484.jpg)

If we then go onto the system and run a command like `stress-ng --all 2` to create some [stress](https://wiki.ubuntu.com/Kernel/Reference/stress-ng), we see some charts begin to have anomaly probabilities that jump outside the typical range. When the anomaly probabilities change enough, we will start seeing anomalies being flagged on the `anomalies.anomaly` chart. The idea is that these charts are the most anomalous right now so could be a good place to start your troubleshooting. 

![netdata-anomalies-collector-abnormal](https://user-images.githubusercontent.com/2178292/100663710-9bd7aa00-334e-11eb-9d14-76fda73bc309.jpg)

Then, as the issue passes, the anomaly probabilities should settle back down into their 'normal' range again. 

![netdata-anomalies-collector-normal-again](https://user-images.githubusercontent.com/2178292/100666681-481a9000-3351-11eb-9979-64728ee2dfb6.jpg)

## Requirements

- This collector will only work with Python 3 and requires the packages below be installed.
- Typically you will not need to do this, but, if needed, to ensure Python 3 is used you can add the below line to the `[plugin:python.d]` section of `netdata.conf`

```conf
[plugin:python.d]
    # update every = 1
    command options = -ppython3
```

Install the required python libraries.

```bash
# become netdata user
sudo su -s /bin/bash netdata
# install required packages for the netdata user
pip3 install --user netdata-pandas==0.0.38 numba==0.50.1 scikit-learn==0.23.2 pyod==0.8.3
```

## Configuration

Install the Python requirements above, enable the collector and restart Netdata.

```bash
cd /etc/netdata/
sudo ./edit-config python.d.conf
# Set `anomalies: no` to `anomalies: yes`
sudo systemctl restart netdata
```

The configuration for the anomalies collector defines how it will behave on your system and might take some experimentation with over time to set it optimally for your node. Out of the box, the config comes with some [sane defaults](https://www.netdata.cloud/blog/redefining-monitoring-netdata/) to get you started that try to balance the flexibility and power of the ML models with the goal of being as cheap as possible in term of cost on the node resources. 

_**Note**: If you are unsure about any of the below configuration options then it's best to just ignore all this and leave the `anomalies.conf` file alone to begin with. Then you can return to it later if you would like to tune things a bit more once the collector is running for a while and you have a feeling for its performance on your node._

Edit the `python.d/anomalies.conf` configuration file using `edit-config` from the your agent's [config
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is usually at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/anomalies.conf
```

The default configuration should look something like this. Here you can see each parameter (with sane defaults) and some information about each one and what it does.

```conf
# -
# JOBS (data collection sources)

# Pull data from local Netdata node.
anomalies:
    name: 'Anomalies'

    # Host to pull data from.
    host: '127.0.0.1:19999'

    # Username and Password for Netdata if using basic auth.
    # username: '???'
    # password: '???'

    # Use http or https to pull data
    protocol: 'http'

    # SSL verify parameter for requests.get() calls
    tls_verify: true

    # What charts to pull data for - A regex like 'system\..*|' or 'system\..*|apps.cpu|apps.mem' etc.
    charts_regex: 'system\..*'

    # Charts to exclude, useful if you would like to exclude some specific charts. 
    # Note: should be a ',' separated string like 'chart.name,chart.name'.
    charts_to_exclude: 'system.uptime,system.entropy'

    # What model to use - can be one of 'pca', 'hbos', 'iforest', 'cblof', 'loda', 'copod' or 'feature_bagging'. 
    # More details here: https://pyod.readthedocs.io/en/latest/pyod.models.html.
    model: 'pca'

    # Max number of observations to train on, to help cap compute cost of training model if you set a very large train_n_secs.
    train_max_n: 100000

    # How often to re-train the model (assuming update_every=1 then train_every_n=1800 represents (re)training every 30 minutes).
    # Note: If you want to turn off re-training set train_every_n=0 and after initial training the models will not be retrained.
    train_every_n: 1800

    # The length of the window of data to train on (14400 = last 4 hours).
    train_n_secs: 14400

    # How many prediction steps after a train event to just use previous prediction value for. 
    # Used to reduce possibility of the training step itself appearing as an anomaly on the charts.
    train_no_prediction_n: 10

    # If you would like to train the model for the first time on a specific window then you can define it using the below two variables.
    # Start of training data for initial model.
    # initial_train_data_after: 1604578857

    # End of training data for initial model.
    # initial_train_data_before: 1604593257

    # If you would like to ignore recent data in training then you can offset it by offset_n_secs.
    offset_n_secs: 0

    # How many lagged values of each dimension to include in the 'feature vector' each model is trained on.
    lags_n: 5

    # How much smoothing to apply to each dimension in the 'feature vector' each model is trained on.
    smooth_n: 3

    # How many differences to take in preprocessing your data. 
    # More info on differencing here: https://en.wikipedia.org/wiki/Autoregressive_integrated_moving_average#Differencing
    # diffs_n=0 would mean training models on the raw values of each dimension.
    # diffs_n=1 means everything is done in terms of differences. 
    diffs_n: 1

    # What is the typical proportion of anomalies in your data on average? 
    # This parameter can control the sensitivity of your models to anomalies. 
    # Some discussion here: https://github.com/yzhao062/pyod/issues/144
    contamination: 0.001

    # Set to true to include an "average_prob" dimension on anomalies probability chart which is 
    # just the average of all anomaly probabilities at each time step
    include_average_prob: true

    # Define any custom models you would like to create anomaly probabilities for, some examples below to show how.
    # For example below example creates two custom models, one to run anomaly detection user and system cpu for our demo servers
    # and one on the cpu and mem apps metrics for the python.d.plugin.
    # custom_models:
    #   - name: 'demos_cpu'
    #     dimensions: 'london.my-netdata.io::system.cpu|user,london.my-netdata.io::system.cpu|system,newyork.my-netdata.io::system.cpu|user,newyork.my-netdata.io::system.cpu|system'
    #   - name: 'apps_python_d_plugin'
    #     dimensions: 'apps.cpu|python.d.plugin,apps.mem|python.d.plugin'

    # Set to true to normalize, using min-max standardization, features used for the custom models. 
    # Useful if your custom models contain dimensions on very different scales an model you use does 
    # not internally do its own normalization. Usually best to leave as false.
    # custom_models_normalize: false
```

## Custom models

In the `anomalies.conf` file you can also define some "custom models" which you can use to group one or more metrics into a single model much like is done by default for the charts you specify. This is useful if you have a handful of metrics that exist in different charts but perhaps are related to the same underlying thing you would like to perform anomaly detection on, for example a specific app or user. 

To define a custom model you would include configuration like below in `anomalies.conf`. By default there should already be some commented out examples in there. 

`name` is a name you give your custom model, this is what will appear alongside any other specified charts in the `anomalies.probability` and `anomalies.anomaly` charts. `dimensions` is a string of metrics you want to include in your custom model. By default the [netdata-pandas](https://github.com/netdata/netdata-pandas) library used to pull the data from Netdata uses a "chart.a|dim.1" type of naming convention in the pandas columns it returns, hence the `dimensions` string should look like "chart.name|dimension.name,chart.name|dimension.name". The examples below hopefully make this clear.

```yaml
custom_models:
   # a model for anomaly detection on the netdata user in terms of cpu, mem, threads, processes and sockets.
 - name: 'user_netdata'
   dimensions: 'users.cpu|netdata,users.mem|netdata,users.threads|netdata,users.processes|netdata,users.sockets|netdata'
   # a model for anomaly detection on the netdata python.d.plugin app in terms of cpu, mem, threads, processes and sockets.
 - name: 'apps_python_d_plugin'
   dimensions: 'apps.cpu|python.d.plugin,apps.mem|python.d.plugin,apps.threads|python.d.plugin,apps.processes|python.d.plugin,apps.sockets|python.d.plugin'

custom_models_normalize: false
```

## Troubleshooting

To see any relevant log messages you can use a command like below.

```bash
`grep 'anomalies' /var/log/netdata/error.log`
```

If you would like to log in as `netdata` user and run the collector in debug mode to see more detail.

```bash
# become netdata user
sudo su -s /bin/bash netdata
# run collector in debug using `nolock` option if netdata is already running the collector itself.
/usr/libexec/netdata/plugins.d/python.d.plugin anomalies debug trace nolock
```

## Deepdive tutorial

If you would like to go deeper on what exactly the anomalies collector is doing under the hood then check out this [deepdive tutorial](https://github.com/netdata/community/blob/main/netdata-agent-api/netdata-pandas/anomalies_collector_deepdive.ipynb) in our community repo where you can play around with some data from our demo servers (or your own if its accessible to you) and work through the calculations step by step.

(Note: as its a Jupyter Notebook it might render a little prettier on [nbviewer](https://nbviewer.jupyter.org/github/netdata/community/blob/main/netdata-agent-api/netdata-pandas/anomalies_collector_deepdive.ipynb))

## Notes

- Python 3 is required as the [`netdata-pandas`](https://github.com/netdata/netdata-pandas) package uses Python async libraries ([asks](https://pypi.org/project/asks/) and [trio](https://pypi.org/project/trio/)) to make asynchronous calls to the [Netdata REST API](https://github.com/netdata/netdata/blob/master/src/web/api/README.md) to get the required data for each chart.
- Python 3 is also required for the underlying ML libraries of [numba](https://pypi.org/project/numba/), [scikit-learn](https://pypi.org/project/scikit-learn/), and [PyOD](https://pypi.org/project/pyod/).
- It may take a few hours or so (depending on your choice of `train_secs_n`) for the collector to 'settle' into it's typical behaviour in terms of the trained models and probabilities you will see in the normal running of your node.
- As this collector does most of the work in Python itself, with [PyOD](https://pyod.readthedocs.io/en/latest/) leveraging [numba](https://numba.pydata.org/) under the hood, you may want to try it out first on a test or development system to get a sense of its performance characteristics on a node similar to where you would like to use it.
- `lags_n`, `smooth_n`, and `diffs_n` together define the preprocessing done to the raw data before models are trained and before each prediction. This essentially creates a [feature vector](https://en.wikipedia.org/wiki/Feature_(machine_learning)#:~:text=In%20pattern%20recognition%20and%20machine,features%20that%20represent%20some%20object.&text=Feature%20vectors%20are%20often%20combined,score%20for%20making%20a%20prediction.) for each chart model (or each custom model). The default settings for these parameters aim to create a rolling matrix of recent smoothed [differenced](https://en.wikipedia.org/wiki/Autoregressive_integrated_moving_average#Differencing) values for each chart. The aim of the model then is to score how unusual this 'matrix' of features is for each chart based on what it has learned as 'normal' from the training data. So as opposed to just looking at the single most recent value of a dimension and considering how strange it is, this approach looks at a recent smoothed window of all dimensions for a chart (or dimensions in a custom model) and asks how unusual the data as a whole looks. This should be more flexible in capturing a wider range of [anomaly types](https://andrewm4894.com/2020/10/19/different-types-of-time-series-anomalies/) and be somewhat more robust to temporary 'spikes' in the data that tend to always be happening somewhere in your metrics but often are not the most important type of anomaly (this is all covered in a lot more detail in the [deepdive tutorial](https://nbviewer.jupyter.org/github/netdata/community/blob/main/netdata-agent-api/netdata-pandas/anomalies_collector_deepdive.ipynb)).
- You can see how long model training is taking by looking in the logs for the collector `grep 'anomalies' /var/log/netdata/error.log | grep 'training'` and you should see lines like `2020-12-01 22:02:14: python.d INFO: anomalies[local] : training complete in 2.81 seconds (runs_counter=2700, model=pca, train_n_secs=14400, models=26, n_fit_success=26, n_fit_fails=0, after=1606845731, before=1606860131).`. 
  - This also gives counts of the number of models, if any, that failed to fit and so had to default back to the DefaultModel (which is currently [HBOS](https://pyod.readthedocs.io/en/latest/_modules/pyod/models/hbos.html)).
  - `after` and `before` here refer to the start and end of the training data used to train the models.
- On a development n1-standard-2 (2 vCPUs, 7.5 GB memory) vm running Ubuntu 18.04 LTS and not doing any work some of the typical performance characteristics we saw from running this collector (with defaults) were:
  - A runtime (`netdata.runtime_anomalies`) of ~80ms when doing scoring and ~3 seconds when training or retraining the models.
  - Typically ~3%-3.5% additional cpu usage from scoring, jumping to ~60% for a couple of seconds during model training.
  - About ~150mb of ram (`apps.mem`) being continually used by the `python.d.plugin`.
- If you activate this collector on a fresh node, it might take a little while to build up enough data to calculate a realistic and useful model.
- Some models like `iforest` can be comparatively expensive (on same n1-standard-2 system above ~2s runtime during predict, ~40s training time, ~50% cpu on both train and predict) so if you would like to use it you might be advised to set a relatively high `update_every` maybe 10, 15 or 30 in `anomalies.conf`.
- Setting a higher `train_every_n` and `update_every` is an easy way to devote less resources on the node to anomaly detection. Specifying less charts and a lower `train_n_secs` will also help reduce resources at the expense of covering less charts and maybe a more noisy model if you set `train_n_secs` to be too small for how your node tends to behave.
- If you would like to enable this on a Raspberry Pi, then check out [this guide](https://github.com/netdata/netdata/blob/master/docs/developer-and-contributor-corner/raspberry-pi-anomaly-detection.md) which will guide you through first installing LLVM.

## Useful links and further reading

- [PyOD documentation](https://pyod.readthedocs.io/en/latest/), [PyOD Github](https://github.com/yzhao062/pyod).
- [Anomaly Detection](https://en.wikipedia.org/wiki/Anomaly_detection) wikipedia page.
- [Anomaly Detection YouTube playlist](https://www.youtube.com/playlist?list=PL6Zhl9mK2r0KxA6rB87oi4kWzoqGd5vp0) maintained by [andrewm4894](https://github.com/andrewm4894/) from Netdata.
- [awesome-TS-anomaly-detection](https://github.com/rob-med/awesome-TS-anomaly-detection) Github list of useful tools, libraries and resources.
- [Mendeley public group](https://www.mendeley.com/community/interesting-anomaly-detection-papers/) with some interesting anomaly detection papers we have been reading.
- Good [blog post](https://www.anodot.com/blog/what-is-anomaly-detection/) from Anodot on time series anomaly detection. Anodot also have some great whitepapers in this space too that some may find useful.
- Novelty and outlier detection in the [scikit-learn documentation](https://scikit-learn.org/stable/modules/outlier_detection.html).

