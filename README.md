# otelx: Unified OpenTelemetry Bootstrap

> 统一封装 OpenTelemetry TracerProvider / Propagator 初始化，支持多种 exporter、资源标签配置，并提供 gRPC / HTTP instrumentation 助手。适用于所有微服务的观测层基础设施。

---

## 1. 为什么需要 otelx？
- **重复劳动**：多个服务（例如 `services/template`、`services/gateway`）各自拼装 exporter、resource、propagator，维护成本高。
- **配置漂移风险**：采样率、环境标签、全局 Propagator 容易不一致，导致链路追踪断裂。
- **框架整合难**：`pkg/grpcx` 等基础库需要一个稳定入口来配置 OTel；现有服务不统一时难以推广。
- **多后端支持需求**：希望在 stdout、本地 OTLP Collector、Cloud Trace 等方案之间自由切换。

`otelx` 提供“一次配置，全局复用”的能力，为项目内所有服务建立统一的可观测性基础。 

---

## 2. 能力概览
- `Setup(ctx, Config, logger, opts...)`：集中构建 `sdktrace.TracerProvider`、`propagation.TextMapPropagator`，可选择是否注册为全局默认。
- 支持 Exporter：`stdout`、`otlp`、`cloudtrace`（结构开放，可扩展 Jaeger/Zipkin）。
- 自动生成标准 Resource 标签：`service.name`、`service.version`、`deployment.environment`，支持自定义标签。
- 可配置采样率、OTLP endpoint、认证 header、是否使用 insecure 连接等参数。
- 提供 gRPC/HTTP helper：`GRPCServerHandler`、`GRPCClientHandler`、`HTTPHandler`、`HTTPTransport`，直接复用官方 instrumentation。
- 统一 Shutdown：退出时调用 `Provider.Shutdown(ctx)` 即可刷新残余 span 并释放 exporter 资源。
- 完整单元测试覆盖：基础配置、全局注册、资源选项、HTTP/gRPC helper 均有测试。

---

## 3. 配置结构
```go
type Config struct {
    ServiceName    string            `json:"serviceName"`
    ServiceVersion string            `json:"serviceVersion"`
    Environment    string            `json:"environment"`

    Exporter      ExporterType       `json:"exporter"` // stdout|otlp|cloudtrace
    SamplingRatio float64            `json:"samplingRatio"`
    Endpoint      string             `json:"endpoint"`
    Insecure      bool               `json:"insecure"`
    GCPProjectID  string             `json:"gcpProjectId"`
    Headers       map[string]string  `json:"headers"`
    ResourceAttrs map[string]string  `json:"resourceAttrs"`
}
```
- `ServiceName` 必填。
- `SamplingRatio` 默认 0.1（10%），范围 [0,1]。
- `Exporter=stdout`：无依赖，适合开发环境。
- `Exporter=otlp`：对接 OTEL Collector / Jaeger / Tempo 等后端，`Endpoint` 支持 `host:port` 或 `https://`。
- `Exporter=cloudtrace`：需提供 `GCPProjectID` 并确保运行环境具备 GCP 凭据。
- `ResourceAttrs` 可补充如 `service.instance.id`、`deployment.region`。

示例（YAML）
```yaml
telemetry:
  serviceName: orders
  serviceVersion: 1.4.2
  environment: staging
  exporter: otlp
  endpoint: otel-collector:4317
  insecure: true
  samplingRatio: 0.2
  resourceAttrs:
    team: payments
```

---

## 4. 快速上手
```go
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
if err != nil {
    logger.Fatal(ctx, "otel setup failed", err)
}
defer prov.Shutdown(context.Background())
```
- `WithGlobal()`：将构建出来的 Provider/Propagator 注册为 `otel` 全局默认。
- `WithPropagator(custom)`：自定义 Propagator（默认使用 `TraceContext + Baggage`）。
- `WithResourceOptions(opts...)`：补充 `resource.Option`，例如注入 Kubernetes 标签。

---

## 5. API 说明
```go
type Provider struct {
    TP         *sdktrace.TracerProvider
    Propagator propagation.TextMapPropagator
    shutdown   func(context.Context) error
}

func (p *Provider) Shutdown(ctx context.Context) error
func Setup(ctx context.Context, cfg Config, logger logx.Logger, opts ...Option) (*Provider, error)
```
### 可选项（Option）
- `WithGlobal()`：自动调用 `otel.SetTracerProvider` / `otel.SetTextMapPropagator`。
- `WithPropagator(p propagation.TextMapPropagator)`：覆盖默认传播器。
- `WithResourceOptions(resource.Option...)`：追加自定义 resource 配置。

---

## 6. gRPC / HTTP 工具
```go
func GRPCServerHandler(opts ...otelgrpc.Option) stats.Handler
func GRPCClientHandler(opts ...otelgrpc.Option) stats.Handler
func HTTPHandler(operation string, handler http.Handler, opts ...otelhttp.Option) http.Handler
func HTTPTransport(base http.RoundTripper, opts ...otelhttp.Option) http.RoundTripper
```
- gRPC：`grpc.WithStatsHandler(otelx.GRPCServerHandler())` / `grpc.WithStatsHandler(otelx.GRPCClientHandler())`。
- HTTP：`otelx.HTTPHandler("operation", mux)` 或 `otelx.HTTPTransport(http.DefaultTransport)`。

---

## 7. 与现有模块集成
1. **`pkg/grpcx`**：要求在 `ServerConfig.Build` 前调用 `otelx.Setup(..., otelx.WithGlobal())`。`Dial` 亦同。
2. **Template 服务**：删除 `internal/infra/otel`，改用统一配置；入口调用 `Setup` 并 `defer provider.Shutdown`。
3. **Gateway 服务**：
   - 用 `otelx.Setup` 替换原 `internal/infra/tracing` 中 exporter/resource 的实现；
   - `port.TracingProvider` 仅保留薄薄的适配层；
   - gRPC Gateway dial 使用 `otelx.GRPCClientHandler()`。

---

## 8. Shutdown 与错误处理
- 始终在主进程退出前调用 `provider.Shutdown(ctx)`（建议带超时）。
- Exporter 初始化失败会返回错误（带 `otlp exporter` / `cloudtrace exporter` 关键字），调用方可选择 fallback 到 stdout。
- `WithGlobal()` 应保证仅调用一次，避免多次覆盖全局。

---

## 9. 路线图
- [ ] 新增 Jaeger / Zipkin exporter。
- [ ] MeterProvider 与 OTLP Metrics 集成。
- [ ] OTel Logs API 封装。
- [ ] 发布 docker-compose 示例，演示 Collector + Tempo + Grafana 配置。

---

## 10. 测试
- `go test ./...` 覆盖配置校验、全局注册、HTTP/gRPC helper 等。
- `TestSetupOTLPExporter` 在短超时时捕捉 OTLP 连接失败，确保错误提示清晰。

---

## 11. 参考资料
- [OpenTelemetry Go](https://opentelemetry.io/docs/instrumentation/go/)
- [otelgrpc instrumentation](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc)
- [Cloud Trace exporter](https://github.com/GoogleCloudPlatform/opentelemetry-operations-go)
