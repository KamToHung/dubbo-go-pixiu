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
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/apache/dubbo-go-pixiu/pkg/common/metric"
	"github.com/apache/dubbo-go-pixiu/pkg/logger"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric/instrument/syncint64"
)

// SystemMetricProvider provides system-level metrics for Pixiu
type SystemMetricProvider struct {
	mu         sync.RWMutex
	ticker     *time.Ticker
	ctx        context.Context
	cancel     context.CancelFunc
	lastUpdate time.Time
	memStats   runtime.MemStats
}

// NewSystemMetricProvider creates a new system metric provider
func NewSystemMetricProvider() *SystemMetricProvider {
	ctx, cancel := context.WithCancel(context.Background())
	return &SystemMetricProvider{
		ctx:    ctx,
		cancel: cancel,
	}
}

func (p *SystemMetricProvider) GetProviderName() string {
	return "system_metrics"
}

func (p *SystemMetricProvider) GetMetricConfigs() []metric.MetricConfig {
	return []metric.MetricConfig{
		{
			Name:        "pixiu_system_memory_heap_used",
			Description: "Current heap memory usage in bytes",
			Type:        metric.Gauge,
			Unit:        "bytes",
		},
		{
			Name:        "pixiu_system_memory_heap_total",
			Description: "Total heap memory allocated in bytes",
			Type:        metric.Gauge,
			Unit:        "bytes",
		},
		{
			Name:        "pixiu_system_goroutines",
			Description: "Current number of goroutines",
			Type:        metric.Gauge,
			Unit:        "1",
		},
		{
			Name:        "pixiu_system_gc_count",
			Description: "Total number of garbage collections",
			Type:        metric.Counter,
			Unit:        "1",
		},
		{
			Name:        "pixiu_system_uptime",
			Description: "System uptime in seconds",
			Type:        metric.Counter,
			Unit:        "s",
		},
	}
}

func (p *SystemMetricProvider) CollectMetrics(instruments map[string]interface{}, attributes []attribute.KeyValue) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Update memory stats
	runtime.ReadMemStats(&p.memStats)

	// Record heap memory usage
	if gauge, ok := instruments["pixiu_system_memory_heap_used"].(syncint64.UpDownCounter); ok {
		gauge.Add(context.Background(), int64(p.memStats.HeapInuse), attributes...)
	}

	// Record total heap memory
	if gauge, ok := instruments["pixiu_system_memory_heap_total"].(syncint64.UpDownCounter); ok {
		gauge.Add(context.Background(), int64(p.memStats.HeapSys), attributes...)
	}

	// Record number of goroutines
	if gauge, ok := instruments["pixiu_system_goroutines"].(syncint64.UpDownCounter); ok {
		gauge.Add(context.Background(), int64(runtime.NumGoroutine()), attributes...)
	}

	// Record GC count
	if counter, ok := instruments["pixiu_system_gc_count"].(syncint64.Counter); ok {
		counter.Add(context.Background(), int64(p.memStats.NumGC), attributes...)
	}

	// Record uptime (approximate)
	if counter, ok := instruments["pixiu_system_uptime"].(syncint64.Counter); ok {
		if !p.lastUpdate.IsZero() {
			elapsed := time.Since(p.lastUpdate).Seconds()
			counter.Add(context.Background(), int64(elapsed), attributes...)
		}
	}

	p.lastUpdate = time.Now()
}

// Start begins periodic metric collection
func (p *SystemMetricProvider) Start(interval time.Duration) {
	p.ticker = time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-p.ticker.C:
				// Metrics will be collected when the filter calls CollectMetrics
				logger.Debug("System metrics collection tick")
			case <-p.ctx.Done():
				return
			}
		}
	}()
}

// Stop stops the metric collection
func (p *SystemMetricProvider) Stop() {
	if p.ticker != nil {
		p.ticker.Stop()
	}
	p.cancel()
}

// CustomApplicationMetricProvider demonstrates how applications can register custom metrics
type CustomApplicationMetricProvider struct {
	requestsByEndpoint map[string]int64
	mu                 sync.RWMutex
}

// NewCustomApplicationMetricProvider creates a new custom application metric provider
func NewCustomApplicationMetricProvider() *CustomApplicationMetricProvider {
	return &CustomApplicationMetricProvider{
		requestsByEndpoint: make(map[string]int64),
	}
}

func (p *CustomApplicationMetricProvider) GetProviderName() string {
	return "custom_application_metrics"
}

func (p *CustomApplicationMetricProvider) GetMetricConfigs() []metric.MetricConfig {
	return []metric.MetricConfig{
		{
			Name:        "pixiu_custom_endpoint_requests",
			Description: "Number of requests per endpoint",
			Type:        metric.Counter,
			Unit:        "1",
		},
		{
			Name:        "pixiu_custom_processing_time",
			Description: "Custom processing time for special operations",
			Type:        metric.Histogram,
			Unit:        "ms",
		},
		{
			Name:        "pixiu_custom_active_connections",
			Description: "Number of active connections",
			Type:        metric.Gauge,
			Unit:        "1",
		},
	}
}

func (p *CustomApplicationMetricProvider) CollectMetrics(instruments map[string]interface{}, attributes []attribute.KeyValue) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Record requests by endpoint
	if counter, ok := instruments["pixiu_custom_endpoint_requests"].(syncint64.Counter); ok {
		for endpoint, count := range p.requestsByEndpoint {
			endpointAttrs := append(attributes, attribute.String("endpoint", endpoint))
			counter.Add(context.Background(), count, endpointAttrs...)
		}
	}

	// Note: For histogram and gauge metrics, you would typically collect them
	// when the actual events occur, not during this periodic collection
}

// RecordEndpointRequest records a request for a specific endpoint
func (p *CustomApplicationMetricProvider) RecordEndpointRequest(endpoint string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requestsByEndpoint[endpoint]++
}

// RecordProcessingTime records custom processing time
func (p *CustomApplicationMetricProvider) RecordProcessingTime(duration time.Duration, instruments map[string]interface{}, attributes []attribute.KeyValue) {
	if histogram, ok := instruments["pixiu_custom_processing_time"].(syncint64.Histogram); ok {
		histogram.Record(context.Background(), duration.Milliseconds(), attributes...)
	}
}

// UpdateActiveConnections updates the active connections gauge
func (p *CustomApplicationMetricProvider) UpdateActiveConnections(count int64, instruments map[string]interface{}, attributes []attribute.KeyValue) {
	if gauge, ok := instruments["pixiu_custom_active_connections"].(syncint64.UpDownCounter); ok {
		gauge.Add(context.Background(), count, attributes...)
	}
}
