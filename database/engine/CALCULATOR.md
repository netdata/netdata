<!--
title: "Calculate the disk and RAM needed to store your metrics"
description: "This calculator helps you determine how much RAM and disk the Agent's database engine will use based on how long you want to retain your metrics."
custom_edit_url: https://github.com/netdata/netdata/edit/master/database/engine/CALCULATOR.md
-->

<!--
This document uses enhancements only available on Netdata Learn:
https://learn.netdata.cloud/docs/agent/database/calculator
-->

import { Calculator } from '../../../src/components/agent/dbCalc/'

The Agent's database engine uses your system's RAM to store real-time metrics, then "spills" historical metrics to disk
for efficient long-term storage. Read the [database engine docs](/docs/agent/database/engine) for more information.

This calculator helps you determine how much RAM and disk the Agent's database engine will use based on how long you
want to retain your metrics and a few other settings.

> ⚠️ This calculator provides an _estimate_ of disk and RAM usage. Real-life usage may vary based on the accuracy of the
> values you enter below or due to changes in the compression ratio.

<Calculator />