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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	stdhttp "net/http"
	"time"
)

import (
	"github.com/pkg/errors"

	"go.opentelemetry.io/otel/attribute"

	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/metric/instrument"
	"go.opentelemetry.io/otel/metric/instrument/syncint64"
)

import (
	"github.com/apache/dubbo-go-pixiu/pkg/client"
	"github.com/apache/dubbo-go-pixiu/pkg/common/constant"
	"github.com/apache/dubbo-go-pixiu/pkg/common/extension/filter"
	commonMetric "github.com/apache/dubbo-go-pixiu/pkg/common/metric"
	"github.com/apache/dubbo-go-pixiu/pkg/context/http"
	"github.com/apache/dubbo-go-pixiu/pkg/logger"
)

const (
	Kind = constant.HTTPMetricFilter
)

var (
	// Keep existing instruments for backward compatibility
	totalElapsed syncint64.Counter
	totalCount   syncint64.Counter
	totalError   syncint64.Counter
	sizeRequest  syncint64.Counter
	sizeResponse syncint64.Counter
	durationHist syncint64.Histogram

	// Global HTTP metric provider instance
	httpMetricProvider *commonMetric.HTTPMetricProvider
)

func init() {
	filter.RegisterHttpFilter(&Plugin{})

	// Register the default HTTP metric provider
	httpMetricProvider = commonMetric.NewHTTPMetricProvider()
	if err := commonMetric.RegisterProvider(httpMetricProvider); err != nil {
		logger.Errorf("Failed to register HTTP metric provider: %v", err)
	}
}

type (
	// Plugin is http filter plugin.
	Plugin struct {
	}
	// FilterFactory is http filter instance
	FilterFactory struct {
		registryInitialized bool
	}
	Filter struct {
		start     time.Time
		requestID string
	}
	// Config describe the config of FilterFactory
	Config struct{}
)

func (p *Plugin) Kind() string {
	return Kind
}

func (p *Plugin) CreateFilterFactory() (filter.HttpFilterFactory, error) {
	return &FilterFactory{}, nil
}

func (factory *FilterFactory) Config() any {
	return &struct{}{}
}

func (factory *FilterFactory) Apply() error {
	// Initialize the metric registry if not already done
	if !factory.registryInitialized {
		if err := commonMetric.Initialize(); err != nil {
			logger.Errorf("Failed to initialize metric registry: %v", err)
			return err
		}
		factory.registryInitialized = true
		logger.Info("Metric registry initialized successfully")
	}

	// Keep backward compatibility - still register the old metrics
	err := registerOtelMetric()
	if err != nil {
		return err
	}

	// Initialize the metric registry with OpenTelemetry instruments
	return initializeRegistryInstruments()
}

func (factory *FilterFactory) PrepareFilterChain(ctx *http.HttpContext, chain filter.FilterChain) error {
	_ = ctx // unused parameter
	f := &Filter{
		requestID: generateRequestID(),
	}
	chain.AppendDecodeFilters(f)
	chain.AppendEncodeFilters(f)
	return nil
}

func (f *Filter) Decode(c *http.HttpContext) filter.FilterStatus {
	_ = c // we only use f.start in this implementation
	f.start = time.Now()

	// Record request start time in the HTTP metric provider
	if httpMetricProvider != nil {
		httpMetricProvider.RecordRequestStart(f.requestID)
	}

	return filter.Continue
}

func (f *Filter) Encode(c *http.HttpContext) filter.FilterStatus {
	commonAttrs := []attribute.KeyValue{
		attribute.String("code", fmt.Sprintf("%d", c.GetStatusCode())),
		attribute.String("method", c.Request.Method),
		attribute.String("url", c.GetUrl()),
		attribute.String("host", c.Request.Host),
	}

	// Backward compatibility - use old metrics
	latency := time.Since(f.start)
	totalCount.Add(c.Ctx, 1, commonAttrs...)
	latencyMilli := latency.Milliseconds()
	totalElapsed.Add(c.Ctx, latencyMilli, commonAttrs...)
	if c.LocalReply() {
		totalError.Add(c.Ctx, 1)
	}

	durationHist.Record(c.Ctx, latencyMilli, commonAttrs...)
	size, err := computeApproximateRequestSize(c.Request)
	if err != nil {
		logger.Warn("can not compute request size", err)
	} else {
		sizeRequest.Add(c.Ctx, int64(size), commonAttrs...)
	}

	size, err = computeApproximateResponseSize(c.TargetResp)
	if err != nil {
		logger.Warn("can not compute response size", err)
	} else {
		sizeResponse.Add(c.Ctx, int64(size), commonAttrs...)
	}

	// New registry-based metrics collection
	collectRegistryMetrics(c, f.requestID, commonAttrs)

	logger.Debugf("[Metric] [UPSTREAM] receive request | %d | %s | %s | %s | ", c.GetStatusCode(), latency, c.GetMethod(), c.GetUrl())
	return filter.Continue
}

// collectRegistryMetrics collects metrics from all registered providers
func collectRegistryMetrics(c *http.HttpContext, requestID string, commonAttrs []attribute.KeyValue) {
	// Get all registered instruments
	instruments := commonMetric.GetInstruments()
	if len(instruments) == 0 {
		return
	}

	// Create instrument map for providers
	instrumentMap := make(map[string]interface{})
	for name, metricInst := range instruments {
		switch metricInst.Config.Type {
		case commonMetric.Counter:
			if metricInst.Counter != nil {
				instrumentMap[name] = metricInst.Counter
			} else if metricInst.FloatCounter != nil {
				instrumentMap[name] = metricInst.FloatCounter
			}
		case commonMetric.Gauge:
			if metricInst.UpDownCounter != nil {
				instrumentMap[name] = metricInst.UpDownCounter
			} else if metricInst.FloatUpDownCounter != nil {
				instrumentMap[name] = metricInst.FloatUpDownCounter
			}
		case commonMetric.Histogram:
			if metricInst.Histogram != nil {
				instrumentMap[name] = metricInst.Histogram
			} else if metricInst.FloatHistogram != nil {
				instrumentMap[name] = metricInst.FloatHistogram
			}
		}
	}

	// Record metrics using the HTTP metric provider
	if httpMetricProvider != nil {
		httpMetricProvider.RecordRequestMetrics(c, requestID, instrumentMap)
	}

	// Collect from all other registered providers
	commonMetric.CollectAll(commonAttrs)
}

// initializeRegistryInstruments creates OpenTelemetry instruments for all registered metrics
func initializeRegistryInstruments() error {
	meter := global.MeterProvider().Meter("pixiu")
	instruments := commonMetric.GetInstruments()

	for _, metricInst := range instruments {
		config := metricInst.Config

		switch config.Type {
		case commonMetric.Counter:
			counter, err := meter.SyncInt64().Counter(
				config.Name,
				instrument.WithDescription(config.Description),
			)
			if err != nil {
				logger.Errorf("Failed to create counter %s: %v", config.Name, err)
				return err
			}
			metricInst.Counter = counter

		case commonMetric.Gauge:
			upDownCounter, err := meter.SyncInt64().UpDownCounter(
				config.Name,
				instrument.WithDescription(config.Description),
			)
			if err != nil {
				logger.Errorf("Failed to create up-down counter %s: %v", config.Name, err)
				return err
			}
			metricInst.UpDownCounter = upDownCounter

		case commonMetric.Histogram:
			histogram, err := meter.SyncInt64().Histogram(
				config.Name,
				instrument.WithDescription(config.Description),
			)
			if err != nil {
				logger.Errorf("Failed to create histogram %s: %v", config.Name, err)
				return err
			}
			metricInst.Histogram = histogram
		}

		logger.Debugf("Created OpenTelemetry instrument: %s (%s)", config.Name, config.Type)
	}

	return nil
}

// generateRequestID generates a unique request ID
func generateRequestID() string {
	bytes := make([]byte, 8)
	_, _ = rand.Read(bytes) // ignore error for simplicity
	return hex.EncodeToString(bytes)
}

func computeApproximateResponseSize(res any) (int, error) {
	if res == nil {
		return 0, errors.New("client response is nil")
	}
	if unaryResponse, ok := res.(*client.UnaryResponse); ok {
		return len(unaryResponse.Data), nil
	}
	return 0, errors.New("response is not of type client.UnaryResponse")
}

func computeApproximateRequestSize(r *stdhttp.Request) (int, error) {
	if r == nil {
		return 0, errors.New("http.Request is null pointer ")
	}
	s := 0
	if r.URL != nil {
		s = len(r.URL.Path)
	}
	s += len(r.Method)
	s += len(r.Proto)
	for name, values := range r.Header {
		s += len(name)
		for _, value := range values {
			s += len(value)
		}
	}
	s += len(r.Host)
	if r.ContentLength != -1 {
		s += int(r.ContentLength)
	}
	return s, nil
}

func registerOtelMetric() error {
	meter := global.MeterProvider().Meter("pixiu")

	elapsedCounter, err := meter.SyncInt64().Counter("pixiu_request_elapsed", instrument.WithDescription("request total elapsed in pixiu"))
	if err != nil {
		logger.Errorf("register pixiu_request_elapsed metric failed, err: %v", err)
		return err
	}
	totalElapsed = elapsedCounter

	count, err := meter.SyncInt64().Counter("pixiu_request_count", instrument.WithDescription("request total count in pixiu"))
	if err != nil {
		logger.Errorf("register pixiu_request_count metric failed, err: %v", err)
		return err
	}
	totalCount = count

	errorCounter, err := meter.SyncInt64().Counter("pixiu_request_error_count", instrument.WithDescription("request error total count in pixiu"))
	if err != nil {
		logger.Errorf("register pixiu_request_error_count metric failed, err: %v", err)
		return err
	}
	totalError = errorCounter

	sizeRequest, err = meter.SyncInt64().Counter("pixiu_request_content_length", instrument.WithDescription("request total content length in pixiu"))
	if err != nil {
		logger.Errorf("register pixiu_request_content_length metric failed, err: %v", err)
		return err
	}

	sizeResponse, err = meter.SyncInt64().Counter("pixiu_response_content_length", instrument.WithDescription("request total content length response in pixiu"))
	if err != nil {
		logger.Errorf("register pixiu_response_content_length metric failed, err: %v", err)
		return err
	}

	durationHist, err = meter.SyncInt64().Histogram(
		"pixiu_process_time_millicec",
		instrument.WithDescription("request process time response in pixiu"),
	)
	if err != nil {
		logger.Errorf("register pixiu_process_time_millisec metric failed, err: %v", err)
		return err
	}

	return nil
}
