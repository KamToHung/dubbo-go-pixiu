/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package example

import (
	"time"

	"github.com/apache/dubbo-go-pixiu/pkg/common/metric"
	"github.com/apache/dubbo-go-pixiu/pkg/logger"
)

// InitializeExampleProviders demonstrates how to register custom metric providers
func InitializeExampleProviders() {
	// Register system metrics provider
	systemProvider := NewSystemMetricProvider()
	if err := metric.RegisterProvider(systemProvider); err != nil {
		logger.Errorf("Failed to register system metric provider: %v", err)
		return
	}

	// Start system metrics collection with 30 second interval
	systemProvider.Start(30 * time.Second)
	logger.Info("System metric provider registered and started")

	// Register custom application metrics provider
	customProvider := NewCustomApplicationMetricProvider()
	if err := metric.RegisterProvider(customProvider); err != nil {
		logger.Errorf("Failed to register custom application metric provider: %v", err)
		return
	}
	logger.Info("Custom application metric provider registered")

	// Example of how to use the custom provider
	// This would typically be called from your application code
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Simulate some endpoint requests
				customProvider.RecordEndpointRequest("/api/v1/users")
				customProvider.RecordEndpointRequest("/api/v1/orders")
				customProvider.RecordEndpointRequest("/api/v1/products")
			}
		}
	}()
}

// DemonstrateMetricUsage shows how different components can use the metric registry
func DemonstrateMetricUsage() {
	logger.Info("=== Pixiu Metric Registry Usage Examples ===")

	// 1. Register a simple custom provider
	logger.Info("1. Registering custom providers...")
	InitializeExampleProviders()

	// 2. Show how to get all registered providers
	logger.Info("2. Listing all registered providers...")
	providers := metric.GetProviders()
	for name, provider := range providers {
		logger.Infof("   - Provider: %s (%s)", name, provider.GetProviderName())
		configs := provider.GetMetricConfigs()
		for _, config := range configs {
			logger.Infof("     Metric: %s (%s) - %s", config.Name, config.Type, config.Description)
		}
	}

	// 3. Show how to get all registered instruments (after initialization)
	logger.Info("3. Listing all registered instruments...")
	instruments := metric.GetInstruments()
	for name, instrument := range instruments {
		logger.Infof("   - Instrument: %s (%s)", name, instrument.Config.Type)
	}

	logger.Info("=== End of Examples ===")
}
