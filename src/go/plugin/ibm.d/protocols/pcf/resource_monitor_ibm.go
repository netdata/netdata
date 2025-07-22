// SPDX-License-Identifier: GPL-3.0-or-later

package pcf

import (
	"fmt"

	"github.com/ibm-messaging/mq-golang/v5/mqmetric"
)

// ResourceMetric represents a single resource metric from IBM mqmetric
type ResourceMetric struct {
	Class       string
	Type        string
	Element     string
	Instance    string
	Value       interface{}
	MetricName  string
}

// CollectResourceMetrics collects resource metrics using IBM mqmetric library
func (c *Client) CollectResourceMetrics() (map[string]ResourceMetric, error) {
	if !c.metricsReady {
		return nil, fmt.Errorf("resource monitoring not initialized")
	}

	// Get all published metrics from IBM library
	// This uses mqmetric's connection (Connection #2)
	allMetrics := mqmetric.GetPublishedMetrics(mqmetric.GetConnectionKey())

	result := make(map[string]ResourceMetric)

	// Process IBM's metric hierarchy
	for classKey, class := range allMetrics.Classes {
		for typeKey, mType := range class.Types {
			for elemKey, element := range mType.Elements {
				for instanceName, value := range element.Values {
					key := fmt.Sprintf("%s.%s.%s.%s", classKey, typeKey, elemKey, instanceName)
					result[key] = ResourceMetric{
						Class:      class.Name,
						Type:       mType.Name,
						Element:    element.Description,
						Instance:   instanceName,
						Value:      value,
						MetricName: element.MetricName,
					}
				}
			}
		}
	}

	c.protocol.Debugf("collected %d resource metrics", len(result))
	return result, nil
}

// DiscoverAndSubscribeResources discovers and subscribes to resource metrics
func (c *Client) DiscoverAndSubscribeResources(config ResourceDiscoveryConfig) error {
	if !c.metricsReady {
		return fmt.Errorf("resource monitoring not initialized")
	}

	// Configure discovery
	c.resourceConfig.MonitoredQueues.ObjectNames = config.QueueSelector
	c.resourceConfig.MonitoredQueues.UseWildcard = true

	// Discover and subscribe using mqmetric
	err := mqmetric.DiscoverAndSubscribe(*c.resourceConfig)
	if err != nil {
		return fmt.Errorf("failed to discover and subscribe to resources: %w", err)
	}

	c.protocol.Debugf("resource discovery and subscription completed")
	return nil
}

// ResourceDiscoveryConfig configuration for resource discovery
type ResourceDiscoveryConfig struct {
	QueueSelector string
	EnableStats   bool
}

// ResourcePublicationsResult contains the result of resource monitoring queries
type ResourcePublicationsResult struct {
	Stats             CollectionStats
	UserCPUPercent    AttributeValue
	SystemCPUPercent  AttributeValue
	AvailableMemory   AttributeValue
	UsedMemory        AttributeValue
	MemoryUsedMB      AttributeValue
	LogUsedBytes      AttributeValue
	LogMaxBytes       AttributeValue
	// Additional resource metrics can be added here as needed
}

// IsResourceMonitoringSupported checks if resource monitoring is supported
func (c *Client) IsResourceMonitoringSupported() bool {
	// Resource monitoring requires MQ v9+ and is available on distributed platforms
	// For now, we'll assume it's supported if we're connected
	// Real implementation would check the command level and platform
	return c.connected && c.cachedCommandLevel >= 900
}

// EnableResourceMonitoring enables resource monitoring for the queue manager
func (c *Client) EnableResourceMonitoring() error {
	if c.resourceStatus == ResourceStatusFailed {
		return fmt.Errorf("resource monitoring permanently disabled")
	}
	
	if c.resourceStatus == ResourceStatusEnabled {
		// Already enabled
		return nil
	}
	
	// Initialize the metrics connection (Connection #2)
	if !c.metricsReady {
		connConfig := mqmetric.ConnectionConfig{
			ClientMode:      true,
			UserId:          c.config.User,
			Password:        c.config.Password,
			UsePublications: true,
			WaitInterval:    30,
			ConnName:        fmt.Sprintf("%s(%d)", c.config.Host, c.config.Port),
			Channel:         c.config.Channel,
		}
		
		err := mqmetric.InitConnection(c.config.QueueManager, "NETDATA.REPLY.METRICS", "", &connConfig)
		if err != nil {
			c.resourceStatus = ResourceStatusFailed
			return fmt.Errorf("failed to initialize metrics connection: %w", err)
		}
		
		c.metricsReady = true
	}
	
	// Set up basic discovery configuration
	c.resourceConfig = &mqmetric.DiscoverConfig{
		MetaPrefix: "$SYS/MQ/INFO",
	}
	
	c.resourceStatus = ResourceStatusEnabled
	c.protocol.Debugf("resource monitoring enabled")
	return nil
}

// GetResourcePublications retrieves resource monitoring data
func (c *Client) GetResourcePublications() (*ResourcePublicationsResult, error) {
	if c.resourceStatus != ResourceStatusEnabled || !c.metricsReady {
		return nil, fmt.Errorf("resource monitoring not enabled")
	}
	
	// Get metrics from IBM mqmetric library
	metrics, err := c.CollectResourceMetrics()
	if err != nil {
		return nil, fmt.Errorf("failed to collect resource metrics: %w", err)
	}
	
	// Create result structure
	result := &ResourcePublicationsResult{
		Stats: CollectionStats{
			Discovery: struct {
				Success        bool
				AvailableItems int64
				InvisibleItems int64
				IncludedItems  int64
				ExcludedItems  int64
				UnparsedItems  int64
				ErrorCounts    map[int32]int
			}{
				Success:        true,
				AvailableItems: int64(len(metrics)),
				IncludedItems:  int64(len(metrics)),
				ErrorCounts:    make(map[int32]int),
			},
		},
		UserCPUPercent:   NotCollected,
		SystemCPUPercent: NotCollected,
		AvailableMemory:  NotCollected,
		UsedMemory:       NotCollected,
		MemoryUsedMB:     NotCollected,
		LogUsedBytes:     NotCollected,
		LogMaxBytes:      NotCollected,
	}
	
	// Process specific metrics we care about
	for key, metric := range metrics {
		if value, ok := metric.Value.(float64); ok {
			switch {
			case metric.Element == "User CPU time percentage":
				result.UserCPUPercent = AttributeValue(int64(value * 1000)) // Convert to 3 decimal precision
			case metric.Element == "System CPU time percentage":
				result.SystemCPUPercent = AttributeValue(int64(value * 1000))
			case metric.Element == "Available memory":
				result.AvailableMemory = AttributeValue(int64(value))
			case metric.Element == "Used memory":
				result.UsedMemory = AttributeValue(int64(value))
			case metric.Element == "RAM total bytes for queue manager":
				result.MemoryUsedMB = AttributeValue(int64(value / (1024 * 1024))) // Convert bytes to MB
			case metric.Element == "Log - bytes in use":
				result.LogUsedBytes = AttributeValue(int64(value))
			case metric.Element == "Log - bytes max":
				result.LogMaxBytes = AttributeValue(int64(value))
			}
		}
		c.protocol.Debugf("processed resource metric: %s = %v", key, metric.Value)
	}
	
	return result, nil
}

// InitializeResourceDiscovery initializes resource discovery with custom configuration
func (c *Client) InitializeResourceDiscovery(config ResourceDiscoveryConfig) error {
	if !c.metricsReady {
		return fmt.Errorf("resource monitoring connection not available")
	}

	// Configure discovery settings
	discoveryConfig := &mqmetric.DiscoverConfig{
		MetaPrefix: "$SYS/MQ/INFO",
		MonitoredQueues: mqmetric.DiscoverObject{
			ObjectNames:          config.QueueSelector,
			UseWildcard:          true,
			SubscriptionSelector: "",
		},
	}

	// Update connection config if needed
	connConfig := mqmetric.ConnectionConfig{
		ClientMode:      true,
		UserId:          c.config.User,
		Password:        c.config.Password,
		UsePublications: true,
		UseResetQStats:  config.EnableStats,
		WaitInterval:    30,
		ConnName:        fmt.Sprintf("%s(%d)", c.config.Host, c.config.Port),
		Channel:         c.config.Channel,
	}

	// Re-initialize with new configuration
	mqmetric.EndConnection()
	err := mqmetric.InitConnection(c.config.QueueManager, "NETDATA.REPLY.METRICS", "", &connConfig)
	if err != nil {
		c.metricsReady = false
		return fmt.Errorf("failed to reinitialize metrics connection: %w", err)
	}

	// Perform discovery and subscription
	err = mqmetric.DiscoverAndSubscribe(*discoveryConfig)
	if err != nil {
		return fmt.Errorf("failed to discover and subscribe: %w", err)
	}

	c.resourceConfig = discoveryConfig
	c.protocol.Debugf("resource discovery initialized with queue selector: %s", config.QueueSelector)
	
	return nil
}