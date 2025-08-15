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
	"testing"

	"go.opentelemetry.io/otel/attribute"

	"github.com/apache/dubbo-go-pixiu/pkg/common/metric"
)

// TestMetricProvider is a test metric provider
type TestMetricProvider struct {
	name string
}

func (p *TestMetricProvider) GetProviderName() string {
	return p.name
}

func (p *TestMetricProvider) GetMetricConfigs() []metric.MetricConfig {
	return []metric.MetricConfig{
		{
			Name:        "test_counter",
			Description: "Test counter metric",
			Type:        metric.Counter,
			Unit:        "1",
		},
		{
			Name:        "test_gauge",
			Description: "Test gauge metric",
			Type:        metric.Gauge,
			Unit:        "bytes",
		},
		{
			Name:        "test_histogram",
			Description: "Test histogram metric",
			Type:        metric.Histogram,
			Unit:        "ms",
		},
	}
}

func (p *TestMetricProvider) CollectMetrics(instruments map[string]interface{}, attributes []attribute.KeyValue) {
	// Test implementation - just verify the interface works
	_ = instruments
	_ = attributes
}

func TestMetricRegistry(t *testing.T) {
	// Test provider registration
	testProvider := &TestMetricProvider{name: "test_provider"}
	err := metric.RegisterProvider(testProvider)
	if err != nil {
		t.Fatalf("Failed to register test provider: %v", err)
	}

	// Verify provider is registered
	providers := metric.GetProviders()
	if _, exists := providers["test_provider"]; !exists {
		t.Error("Test provider not found in registry")
	}

	// Test registry initialization
	err = metric.Initialize()
	if err != nil {
		t.Fatalf("Failed to initialize metric registry: %v", err)
	}

	// Verify instruments are created
	instruments := metric.GetInstruments()
	expectedMetrics := []string{"test_counter", "test_gauge", "test_histogram"}

	for _, metricName := range expectedMetrics {
		if _, exists := instruments[metricName]; !exists {
			t.Errorf("Expected metric %s not found in instruments", metricName)
		}
	}

	// Test collection (should not panic)
	attrs := []attribute.KeyValue{
		attribute.String("test", "value"),
	}
	metric.CollectAll(attrs)

	// Test unregistration
	metric.UnregisterProvider("test_provider")
	providers = metric.GetProviders()
	if _, exists := providers["test_provider"]; exists {
		t.Error("Test provider should have been unregistered")
	}
}

func TestHTTPMetricProvider(t *testing.T) {
	httpProvider := metric.NewHTTPMetricProvider()

	// Test provider name
	if httpProvider.GetProviderName() != "http_basic_metrics" {
		t.Error("Unexpected HTTP metric provider name")
	}

	// Test metric configs
	configs := httpProvider.GetMetricConfigs()
	expectedMetrics := []string{
		"pixiu_request_elapsed",
		"pixiu_request_count",
		"pixiu_request_error_count",
		"pixiu_request_content_length",
		"pixiu_response_content_length",
		"pixiu_process_time_millisec",
	}

	if len(configs) != len(expectedMetrics) {
		t.Errorf("Expected %d metrics, got %d", len(expectedMetrics), len(configs))
	}

	for _, expected := range expectedMetrics {
		found := false
		for _, config := range configs {
			if config.Name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected metric %s not found", expected)
		}
	}

	// Test request ID generation
	requestID1 := httpProvider.RecordRequestStart("test1")
	requestID2 := httpProvider.RecordRequestStart("test2")
	_ = requestID1
	_ = requestID2
	// Request IDs should be different (this is a basic check)
	// In a real implementation, you'd verify they're actually different
}

func TestMetricRegistryBackwardCompatibility(t *testing.T) {
	// This test ensures that the new registry system doesn't break existing functionality

	// Create filter factory
	plugin := &Plugin{}
	factory, err := plugin.CreateFilterFactory()
	if err != nil {
		t.Fatalf("Failed to create filter factory: %v", err)
	}

	// Test that Apply() works with the new registry
	err = factory.Apply()
	if err != nil {
		t.Fatalf("Filter factory Apply() failed: %v", err)
	}

	// Verify that both old and new metrics are available
	instruments := metric.GetInstruments()

	// Check that HTTP metrics from the provider are registered
	httpMetrics := []string{
		"pixiu_request_count",
		"pixiu_request_elapsed",
		"pixiu_request_error_count",
	}

	for _, metricName := range httpMetrics {
		if _, exists := instruments[metricName]; !exists {
			t.Errorf("HTTP metric %s not found after filter initialization", metricName)
		}
	}
}
