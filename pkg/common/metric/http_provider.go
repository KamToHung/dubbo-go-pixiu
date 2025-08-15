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
	"errors"
	"fmt"
	stdhttp "net/http"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric/instrument/syncint64"

	"github.com/apache/dubbo-go-pixiu/pkg/client"
	"github.com/apache/dubbo-go-pixiu/pkg/context/http"
	"github.com/apache/dubbo-go-pixiu/pkg/logger"
)

// HTTPMetricProvider provides basic HTTP metrics for Pixiu
type HTTPMetricProvider struct {
	startTimes map[string]time.Time
	mu         sync.RWMutex
}

// NewHTTPMetricProvider creates a new HTTP metric provider
func NewHTTPMetricProvider() *HTTPMetricProvider {
	return &HTTPMetricProvider{
		startTimes: make(map[string]time.Time),
	}
}

func (p *HTTPMetricProvider) GetProviderName() string {
	return "http_basic_metrics"
}

func (p *HTTPMetricProvider) GetMetricConfigs() []MetricConfig {
	return []MetricConfig{
		{
			Name:        "pixiu_request_elapsed",
			Description: "request total elapsed in pixiu",
			Type:        Counter,
			Unit:        "ms",
		},
		{
			Name:        "pixiu_request_count",
			Description: "request total count in pixiu",
			Type:        Counter,
			Unit:        "1",
		},
		{
			Name:        "pixiu_request_error_count",
			Description: "request error total count in pixiu",
			Type:        Counter,
			Unit:        "1",
		},
		{
			Name:        "pixiu_request_content_length",
			Description: "request total content length in pixiu",
			Type:        Counter,
			Unit:        "bytes",
		},
		{
			Name:        "pixiu_response_content_length",
			Description: "response total content length in pixiu",
			Type:        Counter,
			Unit:        "bytes",
		},
		{
			Name:        "pixiu_process_time_millisec",
			Description: "request process time response in pixiu",
			Type:        Histogram,
			Unit:        "ms",
		},
	}
}

func (p *HTTPMetricProvider) CollectMetrics(instruments map[string]interface{}, attributes []attribute.KeyValue) {
	// This method will be called by the metric registry to collect metrics
	// The actual metric collection logic will be handled in the filter
	// This is just a placeholder for the interface implementation
	_ = instruments
	_ = attributes
}

// RecordRequestStart records the start time of a request
func (p *HTTPMetricProvider) RecordRequestStart(requestID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.startTimes[requestID] = time.Now()
}

// RecordRequestMetrics records all HTTP request metrics
func (p *HTTPMetricProvider) RecordRequestMetrics(ctx *http.HttpContext, requestID string, instruments map[string]interface{}) {
	p.mu.Lock()
	startTime, exists := p.startTimes[requestID]
	if exists {
		delete(p.startTimes, requestID)
	}
	p.mu.Unlock()

	if !exists {
		logger.Warn("Request start time not found for request ID: %s", requestID)
		startTime = time.Now()
	}

	commonAttrs := []attribute.KeyValue{
		attribute.String("code", fmt.Sprintf("%d", ctx.GetStatusCode())),
		attribute.String("method", ctx.Request.Method),
		attribute.String("url", ctx.GetUrl()),
		attribute.String("host", ctx.Request.Host),
	}

	latency := time.Since(startTime)
	latencyMilli := latency.Milliseconds()

	// Record request count
	if counter, ok := instruments["pixiu_request_count"].(syncint64.Counter); ok {
		counter.Add(ctx.Ctx, 1, commonAttrs...)
	}

	// Record request elapsed time
	if counter, ok := instruments["pixiu_request_elapsed"].(syncint64.Counter); ok {
		counter.Add(ctx.Ctx, latencyMilli, commonAttrs...)
	}

	// Record error count if this is an error response
	if ctx.LocalReply() {
		if counter, ok := instruments["pixiu_request_error_count"].(syncint64.Counter); ok {
			counter.Add(ctx.Ctx, 1, commonAttrs...)
		}
	}

	// Record process time histogram
	if histogram, ok := instruments["pixiu_process_time_millisec"].(syncint64.Histogram); ok {
		histogram.Record(ctx.Ctx, latencyMilli, commonAttrs...)
	}

	// Record request size
	if size, err := computeApproximateRequestSize(ctx.Request); err == nil {
		if counter, ok := instruments["pixiu_request_content_length"].(syncint64.Counter); ok {
			counter.Add(ctx.Ctx, int64(size), commonAttrs...)
		}
	} else {
		logger.Warn("can not compute request size", err)
	}

	// Record response size
	if size, err := computeApproximateResponseSize(ctx.TargetResp); err == nil {
		if counter, ok := instruments["pixiu_response_content_length"].(syncint64.Counter); ok {
			counter.Add(ctx.Ctx, int64(size), commonAttrs...)
		}
	} else {
		logger.Warn("can not compute response size", err)
	}

	logger.Debugf("[Metric] [UPSTREAM] receive request | %d | %s | %s | %s | ", ctx.GetStatusCode(), latency, ctx.GetMethod(), ctx.GetUrl())
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
