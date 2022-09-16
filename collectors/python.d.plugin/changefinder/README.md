<!--
title: "Online change point detection with Netdata"
description: "Use ML-driven change point detection to narrow your focus and shorten root cause analysis."
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/changefinder/README.md
-->

# Online changepoint detection with Netdata

This collector uses the Python [changefinder](https://github.com/shunsukeaihara/changefinder) library to
perform [online](https://en.wikipedia.org/wiki/Online_machine_learning) [changepoint detection](https://en.wikipedia.org/wiki/Change_detection)
on your Netdata charts and/or dimensions.

Instead of this collector just _collecting_ data, it also does some computation on the data it collects to return a
changepoint score for each chart or dimension you configure it to work on. This is
an [online](https://en.wikipedia.org/wiki/Online_machine_learning) machine learning algorithm so there is no batch step
to train the model, instead it evolves over time as more data arrives. That makes this particular algorithm quite cheap
to compute at each step of data collection (see the notes section below for more details) and it should scale fairly
well to work on lots of charts or hosts (if running on a parent node for example).

> As this is a somewhat unique collector and involves often subjective concepts like changepoints and anomalies, we would love to hear any feedback on it from the community. Please let us know on the [community forum](https://community.netdata.cloud/t/changefinder-collector-feedback/972) or drop us a note at [analytics-ml-team@netdata.cloud](mailto:analytics-ml-team@netdata.cloud) for any and all feedback, both positive and negative. This sort of feedback is priceless to help us make complex features more useful.

## Charts

Two charts are available:

### ChangeFinder Scores (`changefinder.scores`)

This chart shows the percentile of the score that is output from the ChangeFinder library (it is turned off by default
but available with `show_scores: true`).

A high observed score is more likely to be a valid changepoint worth exploring, even more so when multiple charts or
dimensions have high changepoint scores at the same time or very close together.

### ChangeFinder Flags (`changefinder.flags`)

This chart shows `1` or `0` if the latest score has a percentile value that exceeds the `cf_threshold` threshold. By
default, any scores that are in the 99th or above percentile will raise a flag on this chart.

The raw changefinder score itself can be a little noisy and so limiting ourselves to just periods where it surpasses
the 99th percentile can help manage the "[signal to noise ratio](https://en.wikipedia.org/wiki/Signal-to-noise_ratio)"
better.

The `cf_threshold` parameter might be one you want to play around with to tune things specifically for the workloads on
your node and the specific charts you want to monitor. For example, maybe the 95th percentile might work better for you
than the 99th percentile.

Below is an example of the chart produced by this collector. The first 3/4 of the period looks normal in that we see a
few individual changes being picked up somewhat randomly over time. But then at around 14:59 towards the end of the
chart we see two periods with 'spikes' of multiple changes for a small period of time. This is the sort of pattern that
might be a sign something on the system that has changed sufficiently enough to merit some investigation.

![changepoint-collector](https://user-images.githubusercontent.com/2178292/108773528-665de980-7556-11eb-895d-798669bcd695.png)

## Requirements

- This collector will only work with Python 3 and requires the packages below be installed.

```bash
# become netdata user
sudo su -s /bin/bash netdata
# install required packages for the netdata user
pip3 install --user numpy==1.19.5 changefinder==0.03 scipy==1.5.4
```

**Note**: if you need to tell Netdata to use Python 3 then you can pass the below command in the python plugin section
of your `netdata.conf` file.

```yaml
[ plugin:python.d ]
  # update every = 1  
  command options = -ppython3
```

## Configuration

Install the Python requirements above, enable the collector and restart Netdata.

```bash
cd /etc/netdata/
sudo ./edit-config python.d.conf
# Set `changefinder: no` to `changefinder: yes`
sudo systemctl restart netdata
```

The configuration for the changefinder collector defines how it will behave on your system and might take some
experimentation with over time to set it optimally for your node. Out of the box, the config comes with
some [sane defaults](https://www.netdata.cloud/blog/redefining-monitoring-netdata/) to get you started that try to
balance the flexibility and power of the ML models with the goal of being as cheap as possible in term of cost on the
node resources.

_**Note**: If you are unsure about any of the below configuration options then it's best to just ignore all this and
leave the `changefinder.conf` file alone to begin with. Then you can return to it later if you would like to tune things
a bit more once the collector is running for a while and you have a feeling for its performance on your node._

Edit the `python.d/changefinder.conf` configuration file using `edit-config` from the your
agent's [config directory](/docs/configure/nodes.md), which is usually at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/changefinder.conf
```

The default configuration should look something like this. Here you can see each parameter (with sane defaults) and some
information about each one and what it does.

```yaml
# ----------------------------------------------------------------------
# JOBS (data collection sources)

# Pull data from local Netdata node.
local:

  # A friendly name for this job.
  name: 'local'

  # What host to pull data from.
  host: '127.0.0.1:19999'

  # What charts to pull data for - A regex like 'system\..*|' or 'system\..*|apps.cpu|apps.mem' etc.
  charts_regex: 'system\..*'

  # Charts to exclude, useful if you would like to exclude some specific charts. 
  # Note: should be a ',' separated string like 'chart.name,chart.name'.
  charts_to_exclude: ''

  # Get ChangeFinder scores 'per_dim' or 'per_chart'.
  mode: 'per_chart'

  # Default parameters that can be passed to the changefinder library.
  cf_r: 0.5
  cf_order: 1
  cf_smooth: 15

  # The percentile above which scores will be flagged.
  cf_threshold: 99

  # The number of recent scores to use when calculating the percentile of the changefinder score.
  n_score_samples: 14400

  # Set to true if you also want to chart the percentile scores in addition to the flags.
  # Mainly useful for debugging or if you want to dive deeper on how the scores are evolving over time.
  show_scores: false
```

## Troubleshooting

To see any relevant log messages you can use a command like below.

```bash
grep 'changefinder' /var/log/netdata/error.log
```

If you would like to log in as `netdata` user and run the collector in debug mode to see more detail.

```bash
# become netdata user
sudo su -s /bin/bash netdata
# run collector in debug using `nolock` option if netdata is already running the collector itself.
/usr/libexec/netdata/plugins.d/python.d.plugin changefinder debug trace nolock
```

## Notes

- It may take an hour or two (depending on your choice of `n_score_samples`) for the collector to 'settle' into it's
  typical behaviour in terms of the trained models and scores you will see in the normal running of your node. Mainly
  this is because it can take a while to build up a proper distribution of previous scores in over to convert the raw
  score returned by the ChangeFinder algorithm into a percentile based on the most recent `n_score_samples` that have
  already been produced. So when you first turn the collector on, it will have a lot of flags in the beginning and then
  should 'settle down' once it has built up enough history. This is a typical characteristic of online machine learning
  approaches which need some initial window of time before they can be useful.
- As this collector does most of the work in Python itself, you may want to try it out first on a test or development
  system to get a sense of its performance characteristics on a node similar to where you would like to use it.
- On a development n1-standard-2 (2 vCPUs, 7.5 GB memory) vm running Ubuntu 18.04 LTS and not doing any work some of the
  typical performance characteristics we saw from running this collector (with defaults) were:
    - A runtime (`netdata.runtime_changefinder`) of ~30ms.
    - Typically ~1% additional cpu usage.
    - About ~85mb of ram (`apps.mem`) being continually used by the `python.d.plugin` under default configuration.

## Useful links and further reading

- [PyPi changefinder](https://pypi.org/project/changefinder/) reference page.
- [GitHub repo](https://github.com/shunsukeaihara/changefinder) for the changefinder library.
- Relevant academic papers:
    - Yamanishi K, Takeuchi J. A unifying framework for detecting outliers and change points from nonstationary time
      series data. 8th ACM SIGKDD international conference on Knowledge discovery and data mining - KDD02. 2002:
      676. ([pdf](https://citeseerx.ist.psu.edu/viewdoc/download?doi=10.1.1.12.3469&rep=rep1&type=pdf))
    - Kawahara Y, Sugiyama M. Sequential Change-Point Detection Based on Direct Density-Ratio Estimation. SIAM
      International Conference on Data Mining. 2009:
      389–400. ([pdf](https://onlinelibrary.wiley.com/doi/epdf/10.1002/sam.10124))
    - Liu S, Yamada M, Collier N, Sugiyama M. Change-point detection in time-series data by relative density-ratio
      estimation. Neural Networks. Jul.2013 43:72–83. [PubMed: 23500502] ([pdf](https://arxiv.org/pdf/1203.0453.pdf))
    - T. Iwata, K. Nakamura, Y. Tokusashi, and H. Matsutani, “Accelerating Online Change-Point Detection Algorithm using
      10 GbE FPGA NIC,” Proc. International European Conference on Parallel and Distributed Computing (Euro-Par’18)
      Workshops, vol.11339, pp.506–517, Aug.
      2018 ([pdf](https://www.arc.ics.keio.ac.jp/~matutani/papers/iwata_heteropar2018.pdf))
- The [ruptures](https://github.com/deepcharles/ruptures) python package is also a good place to learn more about
  changepoint detection (mostly offline as opposed to online but deals with similar concepts).
- A nice [blog post](https://techrando.com/2019/08/14/a-brief-introduction-to-change-point-detection-using-python/)
  showing some of the other options and libraries for changepoint detection in Python.
- [Bayesian changepoint detection](https://github.com/hildensia/bayesian_changepoint_detection) library - we may explore
  implementing a collector for this or integrating this approach into this collector at a future date if there is
  interest and it proves computationaly feasible.
- You might also find the
  Netdata [anomalies collector](https://github.com/netdata/netdata/tree/master/collectors/python.d.plugin/anomalies)
  interesting.
- [Anomaly Detection](https://en.wikipedia.org/wiki/Anomaly_detection) wikipedia page.
- [Anomaly Detection YouTube playlist](https://www.youtube.com/playlist?list=PL6Zhl9mK2r0KxA6rB87oi4kWzoqGd5vp0)
  maintained by [andrewm4894](https://github.com/andrewm4894/) from Netdata.
- [awesome-TS-anomaly-detection](https://github.com/rob-med/awesome-TS-anomaly-detection) Github list of useful tools,
  libraries and resources.
- [Mendeley public group](https://www.mendeley.com/community/interesting-anomaly-detection-papers/) with some
  interesting anomaly detection papers we have been reading.
- Good [blog post](https://www.anodot.com/blog/what-is-anomaly-detection/) from Anodot on time series anomaly detection.
  Anodot also have some great whitepapers in this space too that some may find useful.
- Novelty and outlier detection in
  the [scikit-learn documentation](https://scikit-learn.org/stable/modules/outlier_detection.html).

