# otelx: Unified OpenTelemetry Bootstrap

> 所有服务共享的 OpenTelemetry 初始化模块，提供统一的 TracerProvider/Propagator 构建、Exporter 配置以及 gRPC/HTTP 辅助。

---

## 1. Why otelx?
- **重复劳动**：`services/template`、`services/gateway` 各自实现 OTLP / Cloud Trace 初始化，逻辑分散。
- **缺乏一致性**：采样率、资源标签、Propagator 设置容易漂移。
- **框架耦合样板多**：`pkg/grpcx` 依赖调用侧提前配置 Provider，否则 `otelgrpc` handler 无法生成有效 Trace。
- **生产要求**：需要支持 stdout、OTLP、Cloud Trace 等多种导出方式，且能平滑扩展。

`otelx` 目标是在公共库内提供“一次配置，全局可用”的 OpenTelemetry 启动方案。

---

## 2. Features
- 单一入口 `Setup(ctx, Config, logger, opts...)` 构建 `TracerProvider`、`Propagator`，并可根据选项注册全局。
- 支持多种 Exporter：`stdout`、`otlp`、`cloudtrace`（可扩展 Jaeger 等）。
- 统一 Resource：默认写入 `service.name`、`service.version`、`deployment.environment` 等属性。
- 可配置采样率、Exporter Endpoint、证书、额外 Header/Resource 标签。
- 提供 gRPC/HTTP helper：`GRPCServerHandler()`、`HTTPMiddleware()` 等，直接复用官方 instrumentation。
- 线程安全、支持上下文取消；Shutdown 包装 `TracerProvider`，应用退出时可主动刷新剩余 span。

---

## 3. Config Schema
```go
// exporter 类型；可扩展 Jaeger、Zipkin 等
const (
    ExporterSTDOUT    ExporterType = "stdout"
    ExporterOTLP      ExporterType = "otlp"
    ExporterCloudTrace ExporterType = "cloudtrace"
)

type Config struct {
    ServiceName     string            `json:"serviceName"`
    ServiceVersion  string            `json:"serviceVersion"`
    Environment     string            `json:"environment"` // e.g. dev/staging/prod
    Exporter        ExporterType      `json:"exporter"`
    SamplingRatio   float64           `json:"samplingRatio"`
    Endpoint        string            `json:"endpoint"`
    Insecure        bool              `json:"insecure"`
    GCPProjectID    string            `json:"gcpProjectId"`
    Headers         map[string]string `json:"headers"`
    ResourceAttrs   map[string]string `json:"resourceAttrs"`
}
```

- `ServiceName` 必填；用于 Resource `service.name`。
- `SamplingRatio` 默认 `0.1`（10%）；范围 [0,1]。
- `Exporter=stdout`：用于本地调试，Span 打印到控制台。
- `Exporter=otlp`：配合 OTEL Collector / Jaeger / Tempo；`Endpoint` 可写 `hostname:4317`（支持 `http(s)://`）。
- `Exporter=cloudtrace`：需提供 `GCPProjectID`，自动使用默认凭证。
- `ResourceAttrs`：附加标签，如 `service.instance.id`、`deployment.region`。

配置可通过 YAML/JSON/env 加载，例如：
```yaml
telemetry:
  serviceName: template
  serviceVersion: 1.2.3
  environment: dev
  exporter: otlp
  endpoint: otel-collector:4317
  insecure: true
  samplingRatio: 0.2
```

---

## 4. Quick Start
```go
ctx := context.Background()
telemetryCfg := otelx.Config{
    ServiceName:    "template",
    ServiceVersion: "1.0.0",
    Environment:    "dev",
    Exporter:       otelx.ExporterOTLP,
    Endpoint:       "localhost:4317",
    Insecure:       true,
    SamplingRatio:  0.1,
}
prov, err := otelx.Setup(ctx, telemetryCfg, logger, otelx.WithGlobal())
if err != nil { log.Fatal(err) }
defer prov.Shutdown(context.Background())

serverCfg := grpcx.ServerConfig{
    GRPCListen: "127.0.0.1:9090",
    HTTPListen: "127.0.0.1:8080",
    Logger:     logger,
    // ...
}
server, _ := serverCfg.Build(ctx)
server.InitializeMetrics()
```
- `WithGlobal()` 选项会自动调用 `otel.SetTracerProvider` / `otel.SetTextMapPropagator`。
- 也可只获取 Provider 而不设置全局，供特定组件使用。

---

## 5. API 设计
```go
type Provider struct {
    TP         *sdktrace.TracerProvider
    Propagator propagation.TextMapPropagator
    ShutdownFn func(context.Context) error
}

func (p *Provider) Shutdown(ctx context.Context) error {
    if p.ShutdownFn != nil {
        return p.ShutdownFn(ctx)
    }
    return nil
}

// Setup initializes provider/propagator according to Config and Options.
func Setup(ctx context.Context, cfg Config, logger logx.Logger, opts ...Option) (*Provider, error)
```

### Options
- `WithGlobal()`：调用 `otel.SetTracerProvider`、`otel.SetTextMapPropagator`。
- `WithPropagator(propagation.TextMapPropagator)`：覆盖默认 Propagator。
- `WithResource(resource.Option...)`：附加 Resource 属性。
- `WithMeterProvider()`、`WithLoggerProvider()`：预留度量/日志集成位置（未来扩展）。

### Exporter 插件
`Setup` 根据 `cfg.Exporter` 调用对应实现：
- `setupStdoutExporter()` → `stdouttrace.New()`
- `setupOTLPExporter()` → `otlptracegrpc.NewClient()` / `otlptracegrpc.New(ctx, options...)`
- `setupCloudTraceExporter()` → `cloudtrace.New(projectID, opts...)`

Exporter 失败时，返回错误；外层可决定是否 fallback 到 stdout。

---

## 6. gRPC & HTTP Helper
```go
func GRPCServerHandler(options ...otelgrpc.Option) grpc.StatsHandler
func GRPCClientHandler(options ...otelgrpc.Option) grpc.StatsHandler

func HTTPMiddleware(handler http.Handler) http.Handler
func HTTPTransport(base http.RoundTripper) http.RoundTripper
```
- 直接返回官方 `otelgrpc.NewServerHandler()` 等实例，保证与 `Setup` 建立的 Provider 协同工作。
- HTTP helper 使用 `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp`。

---

## 7. Integration Guide

### 7.1 `pkg/grpcx`
- 在 README 中增加提示：使用 `ServerConfig.Build` 前调用 `otelx.Setup(..., WithGlobal())`。
- 若需要将 `Propagator` 传入日志上下文，可调用 `provider.Propagator`。

### 7.2 Template Service
1. 在 `cmd/server/main.go` 加载 Telemetry 配置；调用 `otelx.Setup`。
2. 移除旧的 `internal/infra/otel` 包。
3. 保持 `grpcx` 构建流程不变，验证 trace 已写入后端。

### 7.3 Gateway Service
1. 用 `otelx.Setup` 替换 `internal/infra/tracing/factory.go` 里的 exporter/resource 创建。
2. `NewOTelTracingProvider` 从 `Provider` 中取 `TP`、`Propagator`；继续适配 `port.TracingProvider`。
3. `grpc.WithStatsHandler(otelgrpc.NewClientHandler())` → `grpc.WithStatsHandler(otelx.GRPCClientHandler())`（语义等价）。
4. 继续复用网关现有 HTTP Trace 中间件，只需 provider 统一。

---

## 8. Shutdown / Error Handling
- 应用退出时调用 `Provider.Shutdown(ctx)`（配合超时 context），保证剩余 span 批量导出。
- 若 exporter 初始化失败，返回错误并提示 fallback；业务可按需降级至 stdout。
- `WithGlobal()` 只调用一次；若需要切换全局 Provider，应在同一进程保持唯一性。

---

## 9. Roadmap
- [ ] 支持 Jaeger / Zipkin exporter。
- [ ] 扩展 MeterProvider（Prometheus OTLP Metrics）。
- [ ] 提供日志（OTel Log API）接入。
- [ ] 发布示例（docker-compose + otel-collector + tempo + grafana）。

---

## 10. References
- OpenTelemetry Go Quick Start – 官方文档
- otelgrpc instrumentation – 官方仓库
- Cloud Trace exporter – GCP 官方 SDK

