# Netdata Cloud On-Prem

Netdata Cloud is built as microservices and is orchestrated by a Kubernetes cluster, providing a highly available and auto-scaled observability platform.

The overall architecture looks like this:

```mermaid
flowchart TD
    Agents("🌍 <b>Netdata Agents</b><br/>Users' infrastructure<br/>Netdata Children & Parents")
    users[["🔥 <b>Unified Dashboards</b><br/>Integrated Infrastructure<br/>Dashboards"]]
    ingress("🛡️ <b>Ingress Gateway</b><br/>TLS termination")
    traefik((("🔒 <b>Traefik</b><br/>Authentication &<br/>Authorization")))
    emqx(("📤 <b>EMQX</b><br/>Agents Communication<br/>Message Bus<br/>MQTT"))
    pulsar(("⚡ <b>Pulsar</b><br/>Internal Microservices<br/>Message Bus"))
    frontend("🌐 <b>Front-End</b><br/>Static Web Files")
    auth("👨‍💼 <b>Users &amp; Agents</b><br/>Authorization<br/>Microservices")
    spaceroom("🏡 <b>Spaces, Rooms,<br/>Nodes, Settings</b><br/>Microservices for<br/>managing Spaces,<br/>Rooms, Nodes and<br/>related settings")
    charts("📈 <b>Metrics & Queries</b><br/>Microservices for<br/>dispatching queries<br/>to Netdata Agents")
    alerts("🔔 <b>Alerts & Notifications</b><br/>Microservices for<br/>tracking alert<br/>transitions and<br/>deduplicating alerts")
    sql[("✨ <b>PostgreSQL</b><br/>Users, Spaces, Rooms,<br/>Agents, Nodes, Metric<br/>Names, Metrics Retention,<br/>Custom Dashboards,<br/>Settings")]
    redis[("🗒️ <b>Redis</b><br/>Caches needed<br/>by Microservices")]
    elk[("🗞️ <b>Elasticsearch</b><br/>Feed Events Database")]
    bridges("🤝 <b>Input & Output</b><br/>Microservices bridging<br/>agents to internal<br/>components")
    notifications("📢 <b>Notifications Integrations</b><br/>Dispatch alert<br/>notifications to<br/>3rd party services")
    feed("📝 <b>Feed & Events</b><br/>Microservices for<br/>managing the events feed")
    users --> ingress
    agents --> ingress
    ingress --> traefik
    ingress ==>|agents<br/>websockets| emqx
    traefik -.- auth
    traefik ==>|http| spaceroom
    traefik ==>|http| frontend
    traefik ==>|http| charts
    traefik ==>|http| alerts
    spaceroom o-...-o pulsar
    spaceroom -.- redis
    spaceroom x-..-x sql
    spaceroom -.-> feed
    charts o-.-o pulsar
    charts -.- redis
    charts x-.-x sql
    charts -..-> feed
    alerts o-.-o pulsar
    alerts -.- redis
    alerts x-.-x sql
    alerts -..-> feed
    auth o-.-o pulsar
    auth -.- redis
    auth x-.-x sql
    auth -.-> feed
    feed <--> elk
    alerts ----> notifications
    %% auth ~~~ spaceroom
    emqx <.-> bridges o-..-o pulsar
```
