![image6](https://cloud.githubusercontent.com/assets/2662304/14253733/53550b16-fa95-11e5-8d9d-4ed171df4735.gif)

---

## Linux Anti-DDoS

SYNPROXY is a TCP SYN packets proxy. It can be used to protect any TCP server (like a web server) from SYN floods and similar DDos attacks.

SYNPROXY is a netfilter module, in the Linux kernel (since version 3.12). It is optimized to handle millions of packets per second utilizing all CPUs available without any concurrency locking between the connections.

The net effect of this, is that the real servers will not notice any change during the attack. The valid TCP connections will pass through and served, while the attack will be stopped at the firewall.

To use SYNPROXY on your firewall, please follow our setup guides:

 - **[Working with SYNPROXY](https://github.com/firehol/firehol/wiki/Working-with-SYNPROXY)**
 - **[Working with SYNPROXY and traps](https://github.com/firehol/firehol/wiki/Working-with-SYNPROXY-and-traps)**

# Real-time monitoring of Linux Anti-DDoS Protection

As of today (Apr 9th, 2016) netdata is able to monitor in real-time (per second updates) the operation of the Linux Anti-DDoS protection.

It visualizes 4 charts:

1. TCP SYN Packets received on ports operated by SYNPROXY
2. TCP Cookies (valid, invalid, retransmits)
3. Connections Reopened
4. Entries used

Example image:

![ddos](https://cloud.githubusercontent.com/assets/2662304/14398891/6016e3fc-fdf0-11e5-942b-55de6a52cb66.gif)

See Linux Anti-DDoS in action at: **[netdata demo site (with SYNPROXY enabled)](http://netdata.firehol.org/#netfilter_synproxy)** (but read the note below).

---

## A note for DDoS testers

Since I posted this, a few folks tried to run DDoS against http://netdata.firehol.org.

Well, guys **this site is not a test bed for DDoS**. Don't do this. We pay for the bandwidth and you are just wasting it!

Also, please try to understand what you are doing. SYNPROXY is about **spoofed packets**. Making a large set of POSTs or instructing exploited wordpress installations to attack the demo site, **is not a DDoS that SYNPROXY can detect**.

Next, http://netdata.firehol.org is behind cloudflare.com, so even if you manage to make a spoofed IPs attack, **you will actually attack cloudflare.com**. Do not expect to see the traffic cloudflare detected as spoofed on the netdata demo site (you are reaching the demo site, through cloudflare proxies). You can see a real attack on the demo site, only if you attack its real IP (but, you guessed it - it only accepts requests from cloudflare.com). **The demo site is just a demo for netdata, not a demo for DDoS**.

And finally, thank you for exposing the IPs and hostnames of the exploited wordpress installations and the IPs of the hosts you manage to instruct them make so many POST requests to us.

Evidence of the attacks:

1. Attack from wordpress installations
 - [ngnix log with the exploited wordpress installations used today](https://iplists.firehol.org/netdata-attacks/wordpress_log-20160409-1816.txt)
 - [IPs of the exploited wordpress installations used today](https://iplists.firehol.org/netdata-attacks/wordpress_ips-20160409-1816.txt)

2. Attack using a large set of POST requests
 - [ngnix log with the POST requests](https://iplists.firehol.org/netdata-attacks/post_log-20160409-1456.txt)
 - [IPs of the hosts that made the POST requests](https://iplists.firehol.org/netdata-attacks/post_ips-20160409-1456.txt)

You actually stressed netdata a bit. What saved the server from your attack was QoS. I had the bandwidth limited to 50Mbps, so your attacks could not bring the server to its limits. Now, I lowered it even more.

This is what happened with the POSTs:

![image](https://cloud.githubusercontent.com/assets/2662304/14405861/ea5b9a48-fea0-11e5-8e24-9eb0506943d5.png)

And this is what happened with the wordpress attack:

![image](https://cloud.githubusercontent.com/assets/2662304/14405871/4d4c00de-fea1-11e5-9575-1fa70e8b1d25.png)

Now our nginx configuration includes these:

```
    if ($http_user_agent ~* "WordPress") {
        return 403;
    }

    if ($request_method !~ ^(GET|HEAD|OPTIONS)$ ) {
        return 403;
    }

    include firehol_webserver.conf;
    include netdata-attacks.conf;
```

`firehol_webserver` is [this IP Blacklist](http://iplists.firehol.org/?ipset=firehol_webserver) and `netdata-attacks` are the IPs given in the evidence above.

So, you are just blacklisted.

---

Other IP lists, matching the attackers IPs:

IP Blacklist|Attacker IPs Matched
:----------:|:------------------:
[firehol_abusers_30d](https://iplists.firehol.org/?ipset=firehol_abusers_30d)|330
[cleantalk_updated_30d](https://iplists.firehol.org/?ipset=cleantalk_updated_30d)|311
[cleantalk_30d](https://iplists.firehol.org/?ipset=cleantalk_30d)|303
[stopforumspam_365d](https://iplists.firehol.org/?ipset=stopforumspam_365d)|297
[cleantalk_updated_7d](https://iplists.firehol.org/?ipset=cleantalk_updated_7d)|293
[stopforumspam_180d](https://iplists.firehol.org/?ipset=stopforumspam_180d)|292
[cleantalk_7d](https://iplists.firehol.org/?ipset=cleantalk_7d)|287
[firehol_anonymous](https://iplists.firehol.org/?ipset=firehol_anonymous)|280
[stopforumspam_90d](https://iplists.firehol.org/?ipset=stopforumspam_90d)|277
[stopforumspam](https://iplists.firehol.org/?ipset=stopforumspam)|275
[firehol_proxies](https://iplists.firehol.org/?ipset=firehol_proxies)|263
[stopforumspam_30d](https://iplists.firehol.org/?ipset=stopforumspam_30d)|242
[proxyrss_30d](https://iplists.firehol.org/?ipset=proxyrss_30d)|237
[proxylists_30d](https://iplists.firehol.org/?ipset=proxylists_30d)|235
[proxyrss_7d](https://iplists.firehol.org/?ipset=proxyrss_7d)|226
[proxylists_7d](https://iplists.firehol.org/?ipset=proxylists_7d)|225
[firehol_abusers_1d](https://iplists.firehol.org/?ipset=firehol_abusers_1d)|220
[cleantalk_1d](https://iplists.firehol.org/?ipset=cleantalk_1d)|200
[cleantalk_updated_1d](https://iplists.firehol.org/?ipset=cleantalk_updated_1d)|199
[proxylists_1d](https://iplists.firehol.org/?ipset=proxylists_1d)|172
[proxyrss_1d](https://iplists.firehol.org/?ipset=proxyrss_1d)|170
[proxyspy_30d](https://iplists.firehol.org/?ipset=proxyspy_30d)|153
[stopforumspam_7d](https://iplists.firehol.org/?ipset=stopforumspam_7d)|144
[sblam](https://iplists.firehol.org/?ipset=sblam)|143
[firehol_webserver](https://iplists.firehol.org/?ipset=firehol_webserver)|133
[dronebl_anonymizers](https://iplists.firehol.org/?ipset=dronebl_anonymizers)|129
[pushing_inertia_blocklist](https://iplists.firehol.org/?ipset=pushing_inertia_blocklist)|124
[ri_web_proxies_30d](https://iplists.firehol.org/?ipset=ri_web_proxies_30d)|121
[firehol_level4](https://iplists.firehol.org/?ipset=firehol_level4)|120
[proxyspy_7d](https://iplists.firehol.org/?ipset=proxyspy_7d)|115
[proxz_30d](https://iplists.firehol.org/?ipset=proxz_30d)|110
[dronebl_auto_botnets](https://iplists.firehol.org/?ipset=dronebl_auto_botnets)|110
[iblocklist_level3](https://iplists.firehol.org/?ipset=iblocklist_level3)|97
[ib_bluetack_level3](https://iplists.firehol.org/?ipset=ib_bluetack_level3)|97
[proxyrss](https://iplists.firehol.org/?ipset=proxyrss)|95
[proxylists](https://iplists.firehol.org/?ipset=proxylists)|94
[botscout_30d](https://iplists.firehol.org/?ipset=botscout_30d)|90
[jigsaw_malware](https://iplists.firehol.org/?ipset=jigsaw_malware)|68
[ri_web_proxies_7d](https://iplists.firehol.org/?ipset=ri_web_proxies_7d)|62
[ri_connect_proxies_30d](https://iplists.firehol.org/?ipset=ri_connect_proxies_30d)|58
[stopforumspam_1d](https://iplists.firehol.org/?ipset=stopforumspam_1d)|56
[proxyspy_1d](https://iplists.firehol.org/?ipset=proxyspy_1d)|55
[proxz_7d](https://iplists.firehol.org/?ipset=proxz_7d)|50
[sp_anti_infringement](https://iplists.firehol.org/?ipset=sp_anti_infringement)|47
[botscout_7d](https://iplists.firehol.org/?ipset=botscout_7d)|47
[sslproxies_30d](https://iplists.firehol.org/?ipset=sslproxies_30d)|42
[iblocklist_level1](https://iplists.firehol.org/?ipset=iblocklist_level1)|41
[ib_bluetack_level1](https://iplists.firehol.org/?ipset=ib_bluetack_level1)|41
[blocklist_net_ua](https://iplists.firehol.org/?ipset=blocklist_net_ua)|36
[iblocklist_level2](https://iplists.firehol.org/?ipset=iblocklist_level2)|35
[ib_bluetack_level2](https://iplists.firehol.org/?ipset=ib_bluetack_level2)|35
[sorbs_anonymizers](https://iplists.firehol.org/?ipset=sorbs_anonymizers)|31
[ri_connect_proxies_7d](https://iplists.firehol.org/?ipset=ri_connect_proxies_7d)|31
[sorbs_dul](https://iplists.firehol.org/?ipset=sorbs_dul)|30
[firehol_level3](https://iplists.firehol.org/?ipset=firehol_level3)|30
[cleantalk_new_30d](https://iplists.firehol.org/?ipset=cleantalk_new_30d)|30
[xroxy_7d](https://iplists.firehol.org/?ipset=xroxy_7d)|22
[xroxy_30d](https://iplists.firehol.org/?ipset=xroxy_30d)|22
[bds_atif](https://iplists.firehol.org/?ipset=bds_atif)|22
[tor_exits_7d](https://iplists.firehol.org/?ipset=tor_exits_7d)|21
[tor_exits_30d](https://iplists.firehol.org/?ipset=tor_exits_30d)|21
[tor_exits_1d](https://iplists.firehol.org/?ipset=tor_exits_1d)|21
[tor_exits](https://iplists.firehol.org/?ipset=tor_exits)|21
[talosintel_ipfilter](https://iplists.firehol.org/?ipset=talosintel_ipfilter)|21
[snort_ipfilter](https://iplists.firehol.org/?ipset=snort_ipfilter)|21
[iblocklist_bogons](https://iplists.firehol.org/?ipset=iblocklist_bogons)|21
[ib_bluetack_bogons](https://iplists.firehol.org/?ipset=ib_bluetack_bogons)|21
[dm_tor](https://iplists.firehol.org/?ipset=dm_tor)|21
[bm_tor](https://iplists.firehol.org/?ipset=bm_tor)|21
[iblocklist_edu](https://iplists.firehol.org/?ipset=iblocklist_edu)|20
[ib_bluetack_edu](https://iplists.firehol.org/?ipset=ib_bluetack_edu)|20
[ib_onion_router](https://iplists.firehol.org/?ipset=ib_onion_router)|18
[iblocklist_onion_router](https://iplists.firehol.org/?ipset=iblocklist_onion_router)|18
[et_tor](https://iplists.firehol.org/?ipset=et_tor)|18
[proxyspy](https://iplists.firehol.org/?ipset=proxyspy)|17
[iblocklist_rangetest](https://iplists.firehol.org/?ipset=iblocklist_rangetest)|17
[ib_bluetack_rangetest](https://iplists.firehol.org/?ipset=ib_bluetack_rangetest)|17
[proxz_1d](https://iplists.firehol.org/?ipset=proxz_1d)|16
[lashback_ubl](https://iplists.firehol.org/?ipset=lashback_ubl)|16
[botscout_1d](https://iplists.firehol.org/?ipset=botscout_1d)|15
[ri_connect_proxies_1d](https://iplists.firehol.org/?ipset=ri_connect_proxies_1d)|13
[cleantalk](https://iplists.firehol.org/?ipset=cleantalk)|13
[xroxy_1d](https://iplists.firehol.org/?ipset=xroxy_1d)|12
[cleantalk_updated](https://iplists.firehol.org/?ipset=cleantalk_updated)|12
[php_commenters_30d](https://iplists.firehol.org/?ipset=php_commenters_30d)|11
[sslproxies_7d](https://iplists.firehol.org/?ipset=sslproxies_7d)|10
[sorbs_web](https://iplists.firehol.org/?ipset=sorbs_web)|10
[sorbs_recent_spam](https://iplists.firehol.org/?ipset=sorbs_recent_spam)|9
[ri_web_proxies_1d](https://iplists.firehol.org/?ipset=ri_web_proxies_1d)|9
[dronebl_worms_bots](https://iplists.firehol.org/?ipset=dronebl_worms_bots)|9
[dronebl_irc_drones](https://iplists.firehol.org/?ipset=dronebl_irc_drones)|9
[dshield_30d](https://iplists.firehol.org/?ipset=dshield_30d)|8
[ri_connect_proxies](https://iplists.firehol.org/?ipset=ri_connect_proxies)|7
[gofferje_sip](https://iplists.firehol.org/?ipset=gofferje_sip)|7
[sslproxies_1d](https://iplists.firehol.org/?ipset=sslproxies_1d)|6
[nixspam](https://iplists.firehol.org/?ipset=nixspam)|6
[xroxy](https://iplists.firehol.org/?ipset=xroxy)|5
[sorbs_new_spam](https://iplists.firehol.org/?ipset=sorbs_new_spam)|5
[packetmail_ramnode](https://iplists.firehol.org/?ipset=packetmail_ramnode)|5
[maxmind_proxy_fraud](https://iplists.firehol.org/?ipset=maxmind_proxy_fraud)|5
[firehol_level2](https://iplists.firehol.org/?ipset=firehol_level2)|5
[blueliv_crimeserver_online](https://iplists.firehol.org/?ipset=blueliv_crimeserver_online)|5
[myip](https://iplists.firehol.org/?ipset=myip)|4
[cleantalk_new_7d](https://iplists.firehol.org/?ipset=cleantalk_new_7d)|4
[openbl_all](https://iplists.firehol.org/?ipset=openbl_all)|3
[jigsaw_attacks](https://iplists.firehol.org/?ipset=jigsaw_attacks)|3
[iblocklist_spyware](https://iplists.firehol.org/?ipset=iblocklist_spyware)|3
[ib_bluetack_spyware](https://iplists.firehol.org/?ipset=ib_bluetack_spyware)|3
[dshield_7d](https://iplists.firehol.org/?ipset=dshield_7d)|3
[sorbs_noserver](https://iplists.firehol.org/?ipset=sorbs_noserver)|2
[php_commenters_7d](https://iplists.firehol.org/?ipset=php_commenters_7d)|2
[iblocklist_isp_comcast](https://iplists.firehol.org/?ipset=iblocklist_isp_comcast)|2
[iblocklist_isp_charter](https://iplists.firehol.org/?ipset=iblocklist_isp_charter)|2
[iblocklist_isp_att](https://iplists.firehol.org/?ipset=iblocklist_isp_att)|2
[iblocklist_badpeers](https://iplists.firehol.org/?ipset=iblocklist_badpeers)|2
[iblocklist_badpeers](https://iplists.firehol.org/?ipset=iblocklist_badpeers)|2
[iblocklist_ads](https://iplists.firehol.org/?ipset=iblocklist_ads)|2
[ib_isp_comcast](https://iplists.firehol.org/?ipset=ib_isp_comcast)|2
[ib_isp_charter](https://iplists.firehol.org/?ipset=ib_isp_charter)|2
[ib_isp_att](https://iplists.firehol.org/?ipset=ib_isp_att)|2
[ib_bluetack_badpeers](https://iplists.firehol.org/?ipset=ib_bluetack_badpeers)|2
[ib_bluetack_ads](https://iplists.firehol.org/?ipset=ib_bluetack_ads)|2
[greensnow](https://iplists.firehol.org/?ipset=greensnow)|2
[gpf_comics](https://iplists.firehol.org/?ipset=gpf_comics)|2
[dragon_http](https://iplists.firehol.org/?ipset=dragon_http)|2
[cybercrime](https://iplists.firehol.org/?ipset=cybercrime)|2
[cleantalk_new_1d](https://iplists.firehol.org/?ipset=cleantalk_new_1d)|2
[blocklist_de_mail](https://iplists.firehol.org/?ipset=blocklist_de_mail)|2
[blocklist_de](https://iplists.firehol.org/?ipset=blocklist_de)|2
[stopforumspam_toxic](https://iplists.firehol.org/?ipset=stopforumspam_toxic)|1
[sslproxies](https://iplists.firehol.org/?ipset=sslproxies)|1
[sorbs_smtp](https://iplists.firehol.org/?ipset=sorbs_smtp)|1
[socks_proxy_30d](https://iplists.firehol.org/?ipset=socks_proxy_30d)|1
[proxz](https://iplists.firehol.org/?ipset=proxz)|1
[php_harvesters_30d](https://iplists.firehol.org/?ipset=php_harvesters_30d)|1
[nullsecure](https://iplists.firehol.org/?ipset=nullsecure)|1
[ib_org_joost](https://iplists.firehol.org/?ipset=ib_org_joost)|1
[ib_org_blizzard](https://iplists.firehol.org/?ipset=ib_org_blizzard)|1
[iblocklist_proxies](https://iplists.firehol.org/?ipset=iblocklist_proxies)|1
[iblocklist_org_microsoft](https://iplists.firehol.org/?ipset=iblocklist_org_microsoft)|1
[iblocklist_org_joost](https://iplists.firehol.org/?ipset=iblocklist_org_joost)|1
[iblocklist_org_blizzard](https://iplists.firehol.org/?ipset=iblocklist_org_blizzard)|1
[iblocklist_isp_verizon](https://iplists.firehol.org/?ipset=iblocklist_isp_verizon)|1
[ib_isp_verizon](https://iplists.firehol.org/?ipset=ib_isp_verizon)|1
[ib_bluetack_proxies](https://iplists.firehol.org/?ipset=ib_bluetack_proxies)|1
[ib_bluetack_ms](https://iplists.firehol.org/?ipset=ib_bluetack_ms)|1
[hphosts_fsa](https://iplists.firehol.org/?ipset=hphosts_fsa)|1
[hphosts_emd](https://iplists.firehol.org/?ipset=hphosts_emd)|1
[graphiclineweb](https://iplists.firehol.org/?ipset=graphiclineweb)|1
[dshield_1d](https://iplists.firehol.org/?ipset=dshield_1d)|1
[dronebl_ddos_drones](https://iplists.firehol.org/?ipset=dronebl_ddos_drones)|1
[darklist_de](https://iplists.firehol.org/?ipset=darklist_de)|1
[cta_cryptowall](https://iplists.firehol.org/?ipset=cta_cryptowall)|1
[cleantalk_new](https://iplists.firehol.org/?ipset=cleantalk_new)|1
[blueliv_crimeserver_last_7d](https://iplists.firehol.org/?ipset=blueliv_crimeserver_last_7d)|1
[blueliv_crimeserver_last_30d](https://iplists.firehol.org/?ipset=blueliv_crimeserver_last_30d)|1