## Machine learning (ML) powered anomaly detection

### Overview

As of [`v1.32.0`](https://github.com/netdata/netdata/releases/tag/v1.32.0) Netdata comes with some ML capabilites built into it and available to use out of the box with minimal configuration required.

**Note**: This functionality is still under active development and may face breaking changes. It is considered experimental while we dogfood it internally and among early adopters within the Netdata community. We would like to develop and build on these foundational ml capabilities in the open and with the community so if you would like to get involved and help us with some feedback please feel free to email us at analytics-ml-team@netdata.cloud or come join us in the [🤖-ml-powered-monitoring](https://discord.gg/4eRSEUpJnc) channel of the Netdata discord.

Once ml is enabled, Netdata will begin training a model for each dimension. By default this model is a [k-means clustering](https://en.wikipedia.org/wiki/K-means_clustering) model trained on the most recent 4 hours of data. Rather then just using the most recent value of each raw metric the model works on a preprocessed "feature vector" of recent smoothed and differenced values. Once a model is trained, Netdata will begin producing an anomaly score at each second for each dimension. If this "anomaly score" is sufficiently large this is a sign that 

### Configuration

ssss

### Charts

#### xcsdfsd

### Glossary

- `anomaly score`

### Notes

- xxx
- xxx
