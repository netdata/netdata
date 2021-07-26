<!--
title: "Sendmail monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/sendmail/sendmail.md
sidebar_label: "Sendmail"
-->

# Collector for Sendmail 

A simple collector for the 'mailstats -p' command for sendmail, parses prog-, local-, and esmtp-mailer as well as total and tcp-stats.  
Works for the following output format (check mailstats output beforehand): 
Please notice, that netdata need permissions to perform the 'mailstats -p'-command
```
$ mailstats -p
1625774413 1626159038
 0        0          0       24      30421        0       0       0  prog
 3     6057     595865     4384    1003859       56       0       0  local
 5     3972     931233     7383     649313     1356       0       0  esmtp
 T    10029    1527098    11791    1683593     1412       0       0
 C    27553     7292   1412
```

