<!--
title: "Overview"
sidebar_label: "Overview"
custom_edit_url: "https://github.com/netdata/learn/blob/master/docs/concepts/netdata-cloud/overview.md"
sidebar_position: "1900"
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "Concepts/Netdata cloud"
learn_docs_purpose: "Explain the Netdata cloud, operation, principals, purpose, and how Netdata runs it's SAAS Netdata cloud"
learn_repo_doc: "True"
-->


**********************************************************************

Our Machine-Learning-powered guided troubleshooting tools are designed to give you a cutting edge advantage in your troubleshooting battles. 

Netdata's **Metric Correlations** feature uses a Two Sample Kolmogorov-Smirnov test to look for which metrics have a significant distributional change 
around a highlighted window of interest. This can be useful when you are interested in short term “change detection” and want to try answer the 
question “What else changed at the time of this noted incident?"

Netdata’s new **Anomaly Advisor** feature lets you quickly identify potentially anomalous metrics during a particular timeline of interest. This results 
in considerably speeding up your troubleshooting workflow and saving valuable time when faced with an outage or issue you are trying to root cause. 

Anomaly Advisor uses machine learning to detect if any one of the thousands of metrics that Netdata monitors is behaving anomalously. Thousands of 
machine learning models (one per metric) are trained at the edge on the Netdata agent running in your system – preserving privacy by not storing your 
metric data on our servers. And as always, the Netdata agent is incredibly lightweight, even considering the ML training and inference that are required 
by Anomaly Advisor. 

## Learn more 
<Grid columns="2">
  <Box
    title="Guided troubleshooting tools">
    <BoxList>
      <BoxListItem to="https://github.com/netdata/netdata/blob/master/docs/concepts/guided-troubleshooting/machine-learning-powered-anomaly-advisor.md" title="Anamoly Advisor" />
      <BoxListItem to="https://github.com/netdata/netdata/blob/master/docs/concepts/guided-troubleshooting/metric-correlations.md" title="Metrics Correlations" />
    </BoxList>
  </Box>
</Grid>
