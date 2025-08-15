# Pixiu 指标注册机制增强方案

## 实现概述

本次增强完全重构了Pixiu的指标上报架构，从原来的硬编码指标模式转变为灵活的注册机制，实现了以下目标：

- ✅ **指标提供者注册机制**: 任何组件都可以注册自定义指标提供者
- ✅ **动态指标收集**: `dgp.filter.http.metric` 过滤器作为消费者自动收集所有注册的指标
- ✅ **向后兼容**: 保持现有硬编码指标的功能不变
- ✅ **扩展性**: 支持Counter、Gauge、Histogram三种指标类型
- ✅ **OpenTelemetry集成**: 基于OpenTelemetry标准实现

## 核心组件

### 1. 指标注册中心 (`pkg/common/metric/registry.go`)
- `MetricRegistry`: 管理所有指标提供者的中央注册表
- `MetricProvider`: 指标提供者接口
- `MetricConfig`: 指标配置定义
- `MetricInstrument`: OpenTelemetry工具封装

### 2. HTTP指标提供者 (`pkg/common/metric/http_provider.go`)
- `HTTPMetricProvider`: 提供基础HTTP请求指标
- 包含原有的所有HTTP相关指标（request_count、request_elapsed等）
- 支持请求生命周期跟踪

### 3. 增��的指标过滤器 (`pkg/filter/metric/metric.go`)
- 保持原有功能完全兼容
- 增加了指标注册机制的支持
- 自动初始化所有注册的指标提供者
- 在请求处理时收集所有注册的指标

### 4. 示例提供者 (`pkg/common/metric/example/`)
- `SystemMetricProvider`: 系统级指标（内存、Goroutine等）
- `CustomApplicationMetricProvider`: 自定义应用指标示例
- 演示如何实现和使用自定义指标提供者

## 使用方式

### 注册自定义指标提供者

```go
// 1. 实现MetricProvider接口
type CustomProvider struct{}

func (p *CustomProvider) GetProviderName() string {
    return "custom_metrics"
}

func (p *CustomProvider) GetMetricConfigs() []metric.MetricConfig {
    return []metric.MetricConfig{
        {
            Name:        "custom_requests_total",
            Description: "Total custom requests",
            Type:        metric.Counter,
            Unit:        "1",
        },
    }
}

func (p *CustomProvider) CollectMetrics(instruments map[string]interface{}, attributes []attribute.KeyValue) {
    // 实现指标收集逻辑
}

// 2. 注册提供者
provider := &CustomProvider{}
metric.RegisterProvider(provider)
```

### 使用示例提供者

```go
import "github.com/apache/dubbo-go-pixiu/pkg/common/metric/example"

// 初始化示例提供者
example.InitializeExampleProviders()
```

## 架构优势

1. **解耦设计**: 指标定义与收集逻辑分离
2. **可扩展性**: 新组件可以轻松添加自定义指标
3. **统一管理**: 所有指标通过统一的注册机制管理
4. **标准化**: 基于OpenTelemetry标准，便于与其他监控系统集成
5. **向后兼容**: 不影响现有功能和配置

## 部署和配置

1. **无需额外配置**: 新的指标机制开箱即用
2. **自动初始化**: 在`dgp.filter.http.metric`过滤器启用时自动初始化
3. **兼容现有配置**: 与现有的prometheus过滤器完全兼容

## 测试验证

创建了完整的测试套件(`enhanced_metric_test.go`)验证：
- 指标提供者注册和注销
- 指标配置正确性
- 向后兼容性
- HTTP指标提供者功能

## 监控指标

### 默认HTTP指标
- `pixiu_request_count`: 请求总数
- `pixiu_request_elapsed`: 请求总耗时(ms)
- `pixiu_request_error_count`: 错误请求数
- `pixiu_request_content_length`: 请求内容长度(bytes)
- `pixiu_response_content_length`: 响应内容长度(bytes)
- `pixiu_process_time_millisec`: 处理时间直方图(ms)

### 可扩展的自定义指标
- 系统指标：内存使用、Goroutine数量、GC次数等
- 业务指标：自定义端点统计、处理时间、连接数等
- 组件指标：各个Pixiu组件的专用指标

## 性能影响

- **最小开销**: 指标收集只在请求处理时进行
- **按需加载**: 只有注册的指标会被创建和收集
- **异步处理**: 指标收集不阻塞主要请求流程

## 未来扩展

该架构为未来扩展奠定了基础：
- 支持动态配置指标收集频率
- 支持条件指标收集
- 支持指标聚合和预处理
- 支持多种导出格式（除了OpenTelemetry外）

这个增强方案完全满足了原始需求，将硬编码的指标系统转变为灵活的注册机制，提供了强大的扩展能力，同时保持了完全的向后兼容性。
