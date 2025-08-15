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

package metric

import (
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric/instrument/syncfloat64"
	"go.opentelemetry.io/otel/metric/instrument/syncint64"

	"github.com/apache/dubbo-go-pixiu/pkg/logger"
)

// MetricType defines the type of metric
type MetricType string

const (
	Counter   MetricType = "counter"
	Gauge     MetricType = "gauge"
	Histogram MetricType = "histogram"
)

// MetricConfig defines the configuration for a metric
type MetricConfig struct {
	Name        string
	Description string
	Type        MetricType
	Unit        string
	Labels      []string
}

// MetricProvider defines the interface for metric providers
type MetricProvider interface {
	// GetMetricConfigs returns the list of metrics this provider wants to register
	GetMetricConfigs() []MetricConfig
	// CollectMetrics is called when metrics need to be collected, with the registered instruments
	CollectMetrics(instruments map[string]interface{}, attributes []attribute.KeyValue)
	// GetProviderName returns the name of the provider for logging purposes
	GetProviderName() string
}

// MetricInstrument wraps OpenTelemetry metric instruments
type MetricInstrument struct {
	Config             MetricConfig
	Counter            syncint64.Counter
	UpDownCounter      syncint64.UpDownCounter
	Histogram          syncint64.Histogram
	FloatCounter       syncfloat64.Counter
	FloatUpDownCounter syncfloat64.UpDownCounter
	FloatHistogram     syncfloat64.Histogram
}

// MetricRegistry manages metric providers and instruments
type MetricRegistry struct {
	mu          sync.RWMutex
	providers   map[string]MetricProvider
	instruments map[string]*MetricInstrument
	initialized bool
}

var (
	// GlobalRegistry is the global metric registry instance
	GlobalRegistry = NewMetricRegistry()
)

// NewMetricRegistry creates a new metric registry
func NewMetricRegistry() *MetricRegistry {
	return &MetricRegistry{
		providers:   make(map[string]MetricProvider),
		instruments: make(map[string]*MetricInstrument),
		initialized: false,
	}
}

// RegisterProvider registers a metric provider
func (r *MetricRegistry) RegisterProvider(provider MetricProvider) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	providerName := provider.GetProviderName()
	if _, exists := r.providers[providerName]; exists {
		logger.Warnf("Metric provider %s already registered, replacing", providerName)
	}

	r.providers[providerName] = provider
	logger.Infof("Registered metric provider: %s", providerName)

	// If registry is already initialized, initialize this provider immediately
	if r.initialized {
		return r.initializeProvider(provider)
	}

	return nil
}

// UnregisterProvider unregisters a metric provider
func (r *MetricRegistry) UnregisterProvider(providerName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[providerName]; exists {
		delete(r.providers, providerName)
		logger.Infof("Unregistered metric provider: %s", providerName)
	}
}

// GetProviders returns all registered providers
func (r *MetricRegistry) GetProviders() map[string]MetricProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	providers := make(map[string]MetricProvider)
	for name, provider := range r.providers {
		providers[name] = provider
	}
	return providers
}

// GetInstruments returns all registered instruments
func (r *MetricRegistry) GetInstruments() map[string]*MetricInstrument {
	r.mu.RLock()
	defer r.mu.RUnlock()

	instruments := make(map[string]*MetricInstrument)
	for name, instrument := range r.instruments {
		instruments[name] = instrument
	}
	return instruments
}

// Initialize initializes all registered providers with OpenTelemetry instruments
func (r *MetricRegistry) Initialize() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.initialized {
		logger.Warn("Metric registry already initialized")
		return nil
	}

	for _, provider := range r.providers {
		if err := r.initializeProvider(provider); err != nil {
			logger.Errorf("Failed to initialize provider %s: %v", provider.GetProviderName(), err)
			return err
		}
	}

	r.initialized = true
	logger.Info("Metric registry initialized successfully")
	return nil
}

// initializeProvider initializes a single provider (must be called with lock held)
func (r *MetricRegistry) initializeProvider(provider MetricProvider) error {
	configs := provider.GetMetricConfigs()

	for _, config := range configs {
		instrument, err := r.createInstrument(config)
		if err != nil {
			logger.Errorf("Failed to create instrument %s for provider %s: %v",
				config.Name, provider.GetProviderName(), err)
			return err
		}

		r.instruments[config.Name] = instrument
		logger.Debugf("Created instrument %s for provider %s", config.Name, provider.GetProviderName())
	}

	return nil
}

// createInstrument creates an OpenTelemetry instrument based on config
func (r *MetricRegistry) createInstrument(config MetricConfig) (*MetricInstrument, error) {
	// This will be implemented when the metric filter calls Initialize()
	// For now, we just store the config
	return &MetricInstrument{
		Config: config,
	}, nil
}

// CollectAll collects metrics from all providers
func (r *MetricRegistry) CollectAll(commonAttributes []attribute.KeyValue) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !r.initialized {
		logger.Warn("Metric registry not initialized, skipping collection")
		return
	}

	// Create a map of instruments for providers to use
	instrumentMap := make(map[string]interface{})
	for name, instrument := range r.instruments {
		switch instrument.Config.Type {
		case Counter:
			if instrument.Counter != nil {
				instrumentMap[name] = instrument.Counter
			} else if instrument.FloatCounter != nil {
				instrumentMap[name] = instrument.FloatCounter
			}
		case Gauge:
			if instrument.UpDownCounter != nil {
				instrumentMap[name] = instrument.UpDownCounter
			} else if instrument.FloatUpDownCounter != nil {
				instrumentMap[name] = instrument.FloatUpDownCounter
			}
		case Histogram:
			if instrument.Histogram != nil {
				instrumentMap[name] = instrument.Histogram
			} else if instrument.FloatHistogram != nil {
				instrumentMap[name] = instrument.FloatHistogram
			}
		}
	}

	// Collect from all providers
	for _, provider := range r.providers {
		provider.CollectMetrics(instrumentMap, commonAttributes)
	}
}

// Helper functions for easy registration
func RegisterProvider(provider MetricProvider) error {
	return GlobalRegistry.RegisterProvider(provider)
}

func UnregisterProvider(providerName string) {
	GlobalRegistry.UnregisterProvider(providerName)
}

func GetProviders() map[string]MetricProvider {
	return GlobalRegistry.GetProviders()
}

func GetInstruments() map[string]*MetricInstrument {
	return GlobalRegistry.GetInstruments()
}

func Initialize() error {
	return GlobalRegistry.Initialize()
}

func CollectAll(commonAttributes []attribute.KeyValue) {
	GlobalRegistry.CollectAll(commonAttributes)
}
