<!--
---
title: "Anomalies"
custom_edit_url: https://github.com/andrewm4894/netdata/edit/master/collectors/python.d.plugin/anomalies/README.md
---
-->

# Anomalies

Generate Anomaly Scores for specified charts based on models available from the [PyOD](https://pyod.readthedocs.io/en/latest/index.html) library.

The general idea is to view each chart as just a rolling matrix of numbers (one row per line on chart, and a column per timestep) and from that rolling matrix generate an appropriate "[feature vector](https://brilliant.org/wiki/feature-vector/)" (based on typical preprocessing like taking differences, lags, and smoothing etc) to train a PyOD model on.

Then at each timestep the most recent feature vector "X" is used to generate each of:
- **Anomaly Score** - raw anomaly score coming from the trained PyOD model (see PyOD docs [`pyod.models.base.BaseDetector.decision_function`](https://pyod.readthedocs.io/en/latest/api_cc.html#pyod.models.base.BaseDetector.decision_function)).
- **Anomaly Probability** - the anomaly score from above but transformed to by on a `[0,1]` scale and behave more like a probability (see PyOD docs [`pyod.models.base.BaseDetector.decision_function`](https://pyod.readthedocs.io/en/latest/api_cc.html#pyod.models.base.BaseDetector.predict_proba)).
- **Anomaly Flag** - A `1` if the trained PyOD model considered the observation an outlier `0` otherwise (see PyOD docs [`pyod.models.base.BaseDetector.predict`](https://pyod.readthedocs.io/en/latest/api_cc.html#pyod.models.base.BaseDetector.predict)).   

# Useful Links
- [PyOD Examples](https://pyod.readthedocs.io/en/latest/example.html).
- An ["Awesome" list](https://github.com/rob-med/awesome-TS-anomaly-detection) of useful anomaly detection tools and software.
- [Anomaly Detection YouTube Playlist](https://www.youtube.com/playlist?list=PL6Zhl9mK2r0KxA6rB87oi4kWzoqGd5vp0) - Playlist of useful and interesting anomaly detection related videos and talks.

# Feedback

If you have any feedback or want to chat more about anomaly detection or potential other ML features in Netdata please feel free to reach out to us at [analytics-ml-team@netdata.cloud](mailto:analytics-ml-team@netdata.cloud) 
