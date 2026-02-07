// SPDX-License-Identifier: GPL-3.0-or-later

package netflow

type (
	Config struct {
		Vnode       string            `yaml:"vnode,omitempty" json:"vnode"`
		UpdateEvery int               `yaml:"update_every,omitempty" json:"update_every"`
		Address     string            `yaml:"address" json:"address"`
		Protocols   ProtocolsConfig   `yaml:"protocols,omitempty" json:"protocols"`
		Aggregation AggregationConfig `yaml:"aggregation,omitempty" json:"aggregation"`
		Sampling    SamplingConfig    `yaml:"sampling,omitempty" json:"sampling"`
		Exporters   []ExporterConfig  `yaml:"exporters,omitempty" json:"exporters"`
	}

	ProtocolsConfig struct {
		NetFlowV5 bool `yaml:"netflow_v5" json:"netflow_v5"`
		NetFlowV9 bool `yaml:"netflow_v9" json:"netflow_v9"`
		IPFIX     bool `yaml:"ipfix" json:"ipfix"`
		SFlow     bool `yaml:"sflow" json:"sflow"`
	}

	AggregationConfig struct {
		BucketSeconds int `yaml:"bucket_seconds,omitempty" json:"bucket_seconds"`
		MaxBuckets    int `yaml:"max_buckets,omitempty" json:"max_buckets"`
		MaxKeys       int `yaml:"max_keys,omitempty" json:"max_keys"`
		MaxPacketSize int `yaml:"max_packet_size,omitempty" json:"max_packet_size"`
		ReceiveBuffer int `yaml:"receive_buffer,omitempty" json:"receive_buffer"`
	}

	SamplingConfig struct {
		DefaultRate int `yaml:"default_rate,omitempty" json:"default_rate"`
	}

	ExporterConfig struct {
		IP           string `yaml:"ip" json:"ip"`
		Name         string `yaml:"name,omitempty" json:"name"`
		SamplingRate int    `yaml:"sampling_rate,omitempty" json:"sampling_rate"`
	}
)
