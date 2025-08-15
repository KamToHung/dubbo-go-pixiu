# Pixiu 指标注册机制增强

## 概述

Pixiu 的指标上报能力已经得到显著增强。之前指标是硬编码的，现在实现了一个灵活的注册机制，允许 Pixiu 系统中的任何组件注册自定义指标，而 `dgp.filter.http.metric` 过滤器作为消费者导出所有注册的指标。

## 架构设计

### 核心组件

1. **MetricRegistry**: 指标注册中心，管理所有指标提供者
2. **MetricProvider**: 指标提供者接口，任何组件都可以实现此接口来提供指标
3. **MetricConfig**: 指标配置，定义指标的名称、类型、描述等
4. **MetricInstrument**: 封装 OpenTelemetry 指标工具

### 工作流程

```
1. 各组件实现 MetricProvider 接口
2. 通过 RegisterProvider() 注册指标提供者
3. dgp.filter.http.metric 过滤器初始化时调用 Initialize()
4. 过滤器在请求处理时调用 CollectAll() 收集所有指标
```

## 使用方法

### 1. 实现自定义指标提供者

```go
type CustomMetricProvider struct {
    // 自定义字段
}

func (p *CustomMetricProvider) GetProviderName() string {
    return "custom_provider"
}

func (p *CustomMetricProvider) GetMetricConfigs() []metric.MetricConfig {
    return []metric.MetricConfig{
        {
            Name:        "custom_requests_total",
            Description: "Total number of custom requests",
            Type:        metric.Counter,
            Unit:        "1",
        },
        {
            Name:        "custom_response_time",
            Description: "Custom response time",
            Type:        metric.Histogram,
            Unit:        "ms",
        },
    }
}

func (p *CustomMetricProvider) CollectMetrics(instruments map[string]interface{}, attributes []attribute.KeyValue) {
    // 实现指标收集逻辑
    if counter, ok := instruments["custom_requests_total"].(syncint64.Counter); ok {
        counter.Add(context.Background(), 1, attributes...)
    }
}
```

### 2. 注册指标提供者

```go
// 在组件初始化时注册
provider := &CustomMetricProvider{}
if err := metric.RegisterProvider(provider); err != nil {
    logger.Errorf("Failed to register metric provider: %v", err)
}
```

### 3. 指标类型支持

- **Counter**: 单调递增的计数器
- **Gauge**: 可上升或下降的仪表
- **Histogram**: 直方图，用于测量分布

## 内置指标提供者

### HTTPMetricProvider
提供基础的HTTP请求指标：
- `pixiu_request_count`: 请求总数
- `pixiu_request_elapsed`: 请求总耗时
- `pixiu_request_error_count`: 错误请求数
- `pixiu_request_content_length`: 请求内容长度
- `pixiu_response_content_length`: 响应内容长度
- `pixiu_process_time_millisec`: 处理时间直方图

### SystemMetricProvider (示例)
提供系统级指标：
- `pixiu_system_memory_heap_used`: 堆内存使用量
- `pixiu_system_memory_heap_total`: 堆内存总量
- `pixiu_system_goroutines`: Goroutine数量
- `pixiu_system_gc_count`: GC次数
- `pixiu_system_uptime`: 系统运行时间

## 高级用法

### 动态指标收集

```go
// 在运行时记录特定事件的指标
func RecordCustomEvent(eventType string, value int64) {
    instruments := metric.GetInstruments()
    if counter, ok := instruments["custom_events"].(syncint64.Counter); ok {
        attrs := []attribute.KeyValue{
            attribute.String("event_type", eventType),
        }
        counter.Add(context.Background(), value, attrs...)
    }
}
```

### 条件指标注册

```go
// 根据配置或环境变量决定是否注册某些指标
if config.EnableDetailedMetrics {
    detailedProvider := NewDetailedMetricProvider()
    metric.RegisterProvider(detailedProvider)
}
```

## 与现有系统的兼容性

新的指标注册机制完全向后兼容：
- 现有的硬编码指标仍然工作
- 新注册的指标会额外收集
- 不会影响现有的 prometheus 过滤器

## 最佳实践

1. **命名规范**: 使用 `pixiu_component_metric_name` 的命名格式
2. **资源管理**: 在组件销毁时调用 `UnregisterProvider()`
3. **性能考虑**: 避免在指标收集中执行耗时操作
4. **错误处理**: 指标收集失败不应影响主要业务逻辑

## 示例代码

完整的示例代码可以在以下文件中找到：
- `pkg/common/metric/example/example_providers.go`: 示例指标提供者
- `pkg/common/metric/example/usage.go`: 使用示例

## 配置选项

当前版本的指标注册机制不需要额外配置，但可以通过以下方式扩展：

```yaml
# 未来可能的配置选项
metric:
  enabled: true
  collection_interval: 30s
  providers:
    - name: "system_metrics"
      enabled: true
    - name: "custom_app_metrics" 
      enabled: false
```

这个增强的指标系统为 Pixiu 提供了强大而灵活的监控能力，让开发者可以轻松添加自定义指标而无需修改核心代码。
