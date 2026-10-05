module github.com/netdata/netdata/otel-facade-poc

go 1.27.0

require (
	github.com/netdata/netdata/go/plugins v0.0.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/filelogreceiver v0.157.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver v0.157.0
	go.opentelemetry.io/collector/component v1.63.0
	go.opentelemetry.io/collector/confmap v1.63.0
	go.opentelemetry.io/collector/extension v1.63.0
	go.opentelemetry.io/collector/extension/extensioncapabilities v0.157.0
	go.opentelemetry.io/collector/receiver v1.63.0
	go.opentelemetry.io/proto/otlp v1.11.1
	google.golang.org/grpc v1.84.0
)

require (
	github.com/bmatcuk/doublestar/v4 v4.10.2 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/ebitengine/purego v0.10.0 // indirect
	github.com/elastic/lunes v0.2.2 // indirect
	github.com/expr-lang/expr v1.17.8 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/goccy/go-json v0.10.6 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.31.0 // indirect
	github.com/hashicorp/go-version v1.9.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/jonboulle/clockwork v0.5.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/knadh/koanf/maps v0.1.2 // indirect
	github.com/knadh/koanf/providers/confmap v1.0.0 // indirect
	github.com/knadh/koanf/v2 v2.3.5 // indirect
	github.com/leodido/go-syslog/v4 v4.6.0 // indirect
	github.com/leodido/ragel-machinery v0.0.0-20190525184631-5f46317e436b // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/magefile/mage v1.15.0 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/coreinternal v0.157.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/filter v0.157.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/gopsutilenv v0.157.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/experimentalmetricmetadata v0.157.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza v0.157.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/winperfcounters v0.157.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/prometheus/procfs v0.21.1 // indirect
	github.com/shirou/gopsutil/v4 v4.26.6 // indirect
	github.com/stretchr/testify v1.12.1 // indirect
	github.com/tilinna/clock v1.1.0 // indirect
	github.com/tklauser/go-sysconf v0.3.16 // indirect
	github.com/tklauser/numcpus v0.11.0 // indirect
	github.com/valyala/fastjson v1.6.10 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/collector/confmap/xconfmap v0.157.0 // indirect
	go.opentelemetry.io/collector/consumer v1.63.0 // indirect
	go.opentelemetry.io/collector/consumer/consumererror v0.157.0 // indirect
	go.opentelemetry.io/collector/consumer/consumertest v0.157.0 // indirect
	go.opentelemetry.io/collector/consumer/xconsumer v0.157.0 // indirect
	go.opentelemetry.io/collector/extension/xextension v0.157.0 // indirect
	go.opentelemetry.io/collector/featuregate v1.63.0 // indirect
	go.opentelemetry.io/collector/filter v0.157.0 // indirect
	go.opentelemetry.io/collector/internal/componentalias v0.157.0 // indirect
	go.opentelemetry.io/collector/pdata v1.63.0 // indirect
	go.opentelemetry.io/collector/pdata/pprofile v0.157.0 // indirect
	go.opentelemetry.io/collector/pipeline v1.63.0 // indirect
	go.opentelemetry.io/collector/pipeline/xpipeline v0.157.0 // indirect
	go.opentelemetry.io/collector/receiver/receiverhelper v0.157.0 // indirect
	go.opentelemetry.io/collector/receiver/xreceiver v0.157.0 // indirect
	go.opentelemetry.io/collector/scraper v0.157.0 // indirect
	go.opentelemetry.io/collector/scraper/scraperhelper v0.157.0 // indirect
	go.opentelemetry.io/otel v1.46.0 // indirect
	go.opentelemetry.io/otel/metric v1.46.0 // indirect
	go.opentelemetry.io/otel/trace v1.46.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.28.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	gonum.org/v1/gonum v0.17.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260928230214-8a89bd6388cc // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260928230214-8a89bd6388cc // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)

replace github.com/netdata/netdata/go/plugins => ../../src/go
