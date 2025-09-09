module github.com/netdata/netdata/go/plugins

go 1.24.0

replace github.com/prometheus/prometheus => github.com/prometheus/prometheus v0.302.0

require (
	github.com/DATA-DOG/go-sqlmock v1.5.2
	github.com/Masterminds/sprig/v3 v3.3.0
	github.com/Wing924/ltsv v0.4.0
	github.com/araddon/dateparse v0.0.0-20210429162001-6b43995a97de
	github.com/axiomhq/hyperloglog v0.2.5
	github.com/blang/semver/v4 v4.0.0
	github.com/bmatcuk/doublestar/v4 v4.9.1
	github.com/clbanning/rfile/v2 v2.0.0-20231024120205-ac3fca974b0e
	github.com/cloudflare/cfssl v1.6.5
	github.com/coreos/go-systemd/v22 v22.6.0
	github.com/docker/docker v28.4.0+incompatible
	github.com/facebook/time v0.0.0-20250211113239-e3e1421a0980
	github.com/fsnotify/fsnotify v1.9.0
	github.com/go-ldap/ldap/v3 v3.4.11
	github.com/go-sql-driver/mysql v1.9.3
	github.com/godbus/dbus/v5 v5.1.0
	github.com/gofrs/flock v0.12.1
	github.com/gohugoio/hashstructure v0.5.0
	github.com/golang/mock v1.6.0
	github.com/google/uuid v1.6.0
	github.com/gorcon/rcon v1.4.0
	github.com/gosnmp/gosnmp v1.42.1
	github.com/invopop/jsonschema v0.13.0
	github.com/jackc/pgx/v4 v4.18.3
	github.com/jackc/pgx/v5 v5.7.6
	github.com/jessevdk/go-flags v1.6.1
	github.com/kanocz/fcgi_client v0.0.0-20210113082628-fff85c8adfb7
	github.com/likexian/whois v1.15.6
	github.com/likexian/whois-parser v1.24.20
	github.com/lmittmann/tint v1.1.2
	github.com/mattn/go-isatty v0.0.20
	github.com/mattn/go-xmlrpc v0.0.3
	github.com/miekg/dns v1.1.68
	github.com/mitchellh/go-homedir v1.1.0
	github.com/prometheus-community/pro-bing v0.7.0
	github.com/prometheus/common v0.66.1
	github.com/prometheus/prometheus v2.55.1+incompatible
	github.com/redis/go-redis/v9 v9.13.0
	github.com/sijms/go-ora/v2 v2.9.0
	github.com/sourcegraph/conc v0.3.0
	github.com/stretchr/testify v1.11.1
	github.com/tidwall/gjson v1.18.0
	github.com/valyala/fastjson v1.6.4
	github.com/vmware/govmomi v0.52.0
	go.mongodb.org/mongo-driver v1.17.4
	go.uber.org/automaxprocs v1.6.0
	golang.org/x/net v0.43.0
	golang.org/x/text v0.29.0
	golang.zx2c4.com/wireguard/wgctrl v0.0.0-20220504211119-3d4a969bb56b
	gopkg.in/ini.v1 v1.67.0
	gopkg.in/rethinkdb/rethinkdb-go.v6 v6.2.2
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.34.0
	k8s.io/apimachinery v0.34.0
	k8s.io/client-go v0.34.0
	layeh.com/radius v0.0.0-20190322222518-890bc1058917
)

require (
	dario.cat/mergo v1.0.1 // indirect
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/Azure/go-ntlmssp v0.0.0-20221128193559-754e69321358 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver/v3 v3.3.0 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/containerd/errdefs v1.0.0 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dgryski/go-metro v0.0.0-20180109044635-280f6062b5bc // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/distribution/reference v0.5.0 // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/emicklei/go-restful/v3 v3.12.2 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fxamacker/cbor/v2 v2.9.0 // indirect
	github.com/go-asn1-ber/asn1-ber v1.5.8-0.20250403174932-29230038a667 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/jsonpointer v0.21.0 // indirect
	github.com/go-openapi/jsonreference v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/certificate-transparency-go v1.1.7 // indirect
	github.com/google/gnostic-models v0.7.0 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/grafana/regexp v0.0.0-20240518133315-a468a5bfb3bc // indirect
	github.com/hailocab/go-hostpool v0.0.0-20160125115350-e80d13ce29ed // indirect
	github.com/huandu/xstrings v1.5.0 // indirect
	github.com/jackc/chunkreader/v2 v2.0.1 // indirect
	github.com/jackc/pgconn v1.14.3 // indirect
	github.com/jackc/pgio v1.0.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgproto3/v2 v2.3.3 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgtype v1.14.0 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/josharian/native v1.1.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kamstrup/intmap v0.5.1 // indirect
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/likexian/gokit v0.25.15 // indirect
	github.com/mailru/easyjson v0.9.0 // indirect
	github.com/mdlayher/genetlink v1.3.2 // indirect
	github.com/mdlayher/netlink v1.7.2 // indirect
	github.com/mdlayher/socket v0.4.1 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/moby/sys/atomicwriter v0.1.0 // indirect
	github.com/moby/sys/sequential v0.6.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/montanaflynn/stats v0.7.1 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.2 // indirect
	github.com/opentracing/opentracing-go v1.1.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/spf13/cast v1.7.0 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.0 // indirect
	github.com/wk8/go-ordered-map/v2 v2.1.8 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.1.2 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/youmark/pkcs8 v0.0.0-20240726163527-a2c0da244d78 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.59.0 // indirect
	go.opentelemetry.io/otel v1.34.0 // indirect
	go.opentelemetry.io/otel/metric v1.34.0 // indirect
	go.opentelemetry.io/otel/trace v1.34.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/crypto v0.41.0 // indirect
	golang.org/x/mod v0.27.0 // indirect
	golang.org/x/oauth2 v0.30.0 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/term v0.34.0 // indirect
	golang.org/x/time v0.9.0 // indirect
	golang.org/x/tools v0.36.0 // indirect
	golang.zx2c4.com/wireguard v0.0.0-20230325221338-052af4a8072b // indirect
	google.golang.org/protobuf v1.36.8 // indirect
	gopkg.in/cenkalti/backoff.v2 v2.2.1 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.12.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/kube-openapi v0.0.0-20250710124328-f3f2b991d03b // indirect
	k8s.io/utils v0.0.0-20250604170112-4c0f3b243397 // indirect
	sigs.k8s.io/json v0.0.0-20241014173422-cfa47c3a1cc8 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v6 v6.3.0 // indirect
	sigs.k8s.io/yaml v1.6.0 // indirect
)
