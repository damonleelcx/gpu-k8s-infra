# GPU K8s Infra — 生产级 GPU 调度与任务平台

基于 **Go + Kubernetes** 的 GPU 任务调度基础设施与 Web 控制台，支持提交 GPU Job、查看状态、集成**日志**与 **Prometheus 指标**，并可配合 HPA 与 Grafana 做生产级运维。

---

## 架构概览

```
                    ┌─────────────────┐
                    │   Web 控制台     │  (React/Vite)
                    │  提交任务 / 日志  │
                    └────────┬────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │  GPU Platform   │  (Go API)
                    │  /api/v1/jobs   │
                    │  /metrics       │
                    └────────┬────────┘
                             │
         ┌───────────────────┼───────────────────┐
         ▼                   ▼                   ▼
  ┌─────────────┐   ┌──────────────┐   ┌─────────────────┐
  │ Metrics     │   │ K8s API      │   │ Prometheus      │
  │ Server      │   │ (Job/Pod/Log)│   │ (指标采集)       │
  └─────────────┘   └──────────────┘   └─────────────────┘
         │                   │
         ▼                   ▼
  ┌─────────────┐   ┌──────────────┐
  │ HPA         │   │ GPU Jobs     │
  │ (API 扩容)   │   │ (nvidia.com/gpu)│
  └─────────────┘   └──────────────┘
```

- **API**：提交/列表/删除 GPU Job，拉取 Pod 日志，暴露 `/metrics`。
- **Web**：任务列表、提交表单、日志查看、跳转 Prometheus。
- **K8s**：Job 使用 `nvidia.com/gpu`，需集群已装 **NVIDIA device plugin** 与可选 **metrics-server**（HPA）。

---

## 快速开始

### 1. 本地运行 API（需可访问 K8s 集群）

```bash
# 依赖 Go 1.22+
cd gpu-k8s-infra
go mod download
export GPU_JOBS_NAMESPACE=default   # 可选，默认 default
export KUBECONFIG=/path/to/kubeconfig   # 本地用 kubeconfig
go run ./cmd/api
```

API 默认 `http://localhost:8080`：  
- `GET /api/v1/jobs` — 列表  
- `POST /api/v1/jobs` — 提交  
- `GET /api/v1/jobs/{name}/logs` — 日志  
- `GET /metrics` — Prometheus

### 2. 本地运行 Web 控制台

```bash
cd webapp
npm install
npm run dev
```

浏览器打开 `http://localhost:3000`。Vite 已配置代理：`/api`、`/metrics` 转发到 `localhost:8080`。

### 3. 构建与镜像

```bash
make build          # 产出 bin/api
make build-webapp   # 产出 webapp/dist/
make docker         # 构建 gpu-platform-api:latest
```

### 4. 部署到 Kubernetes

**前置条件**（按需）：  
- 安装 [metrics-server](https://github.com/kubernetes-sigs/metrics-server)（用于 HPA、`kubectl top`）。  
- GPU 节点安装 [NVIDIA device plugin](https://github.com/NVIDIA/k8s-device-plugin)。

```bash
# 部署 namespace、API Deployment/Service、RBAC、HPA
make deploy-base
```

若 API 镜像是私有仓库，请先更新 `deploy/base/api-deployment.yaml` 中的 `image` 并配置 imagePullSecrets。  
API 默认在 `gpu-platform` 命名空间创建并管理同命名空间下的 GPU Job。

---

## 目录结构

```
gpu-k8s-infra/
├── api/v1alpha1/           # GPUInferenceAutoscaler CRD 类型与 DeepCopy
├── cmd/
│   ├── api/                # GPU 任务平台 API
│   └── operator/           # GPU Inference Autoscaler Operator
├── controllers/            # GIA Reconciler
├── pkg/
│   ├── k8s/                # K8s 客户端、GPU Job CRUD、Pod 日志
│   ├── queue/              # 内存任务存储（可换 Redis/DB）
│   ├── metrics/            # Prometheus 指标（API 用）
│   └── autoscaler/         # 扩缩逻辑
│       ├── fetcher/        # Prometheus / Redis 指标
│       ├── predictor/      # 时间序列预测
│       ├── coldstart/      # 冷启动与稳定窗口
│       └── scaler/         # 副本数计算
├── webapp/                 # React + Vite 前端
├── deploy/
│   ├── base/               # API 部署
│   ├── operator/           # CRD、Operator、示例 GIA
│   └── monitoring/         # Prometheus / Grafana
├── Dockerfile
├── Makefile
└── README.md
```

---

## 日志与指标

- **日志**：通过 API `GET /api/v1/jobs/{name}/logs` 拉取对应 Job 下 Pod 的 stdout（基于 K8s Pod Logs API）。  
- **指标**：API 暴露 Prometheus 格式 `/metrics`，包含：  
  - `gpu_platform_api_jobs_total`（按 status）  
  - `gpu_platform_api_jobs_in_flight`  
  - `gpu_platform_api_request_duration_seconds`（按 method、path、status）  

将 Prometheus 指向 API Service（见 `deploy/monitoring/prometheus-scrape.yaml`），并导入 `deploy/monitoring/grafana-dashboard.json` 即可在 Grafana 查看平台概览。

---

## HPA 与生产实践

- 当前 **HPA** 针对 API Deployment，按 CPU 70% 扩容，副本数 1～5。  
- GPU Job 本身由用户按需提交；若需按队列长度或 GPU 利用率自动扩缩，可在此基础上接 **Prometheus Adapter + 自定义指标** 或自研调度逻辑。  

大厂常见做法：  
- API/网关：CPU/内存 HPA。  
- 推理/训练：QPS、队列长度、Kafka lag、GPU 利用率等自定义指标驱动 HPA 或批处理扩容。

---

## GPU Inference Autoscaler Operator（预测 + 冷启动 + QPS/队列/GPU 利用率）

独立 **Controller/Operator**，根据 **队列长度**、**GPU 利用率**、**推理 QPS** 自动扩缩 Deployment，并支持**预测式扩容**与**冷启动**处理。

### 能力概览

| 能力 | 说明 |
|------|------|
| **QPS** | 基于 Prometheus 查询（如 `rate(inference_requests_total[1m])`），按目标 QPS/副本数计算副本 |
| **GPU 利用率** | 基于 DCGM/NVIDIA exporter 等指标，按目标利用率扩缩 |
| **队列长度** | 基于 Redis list/stream/set 长度，按目标条数/副本数扩缩 |
| **预测** | 对指标做时间序列预测（指数平滑/线性），提前扩容（preScaleSeconds） |
| **冷启动** | 预估启动时间、扩缩稳定窗口、可选 warm pool，减少从 0 拉起时的空档 |

### CRD：GPUInferenceAutoscaler

```yaml
apiVersion: autoscaling.gpu.k8s.infra/v1alpha1
kind: GPUInferenceAutoscaler
metadata:
  name: inference-qps
spec:
  scaleTargetRef:
    name: my-gpu-inference   # Deployment
  minReplicas: 0
  maxReplicas: 10
  metrics:
    - type: QPS
      targetPerReplica: 100
      prometheusQuery: 'sum(rate(inference_requests_total{job="my-inference"}[1m]))'
  prediction:
    enable: true
    lookbackWindowSeconds: 300
    preScaleSeconds: 60
    method: exponential
  coldStart:
    estimatedStartupSeconds: 90
    scaleUpDelaySeconds: 120
```

### 构建与运行

```bash
# 构建
make build-operator
# 或镜像
make docker-operator

# 本地跑（需 KUBECONFIG；可选 PROMETHEUS_URL、REDIS_ADDR）
make run-operator
```

### 部署到集群

```bash
# 1. 安装 CRD + Operator（会创建 gpu-autoscaler 命名空间）
make deploy-operator

# 2. 配置 Prometheus/Redis：编辑 deploy/operator/operator.yaml 中的 PROMETHEUS_URL、REDIS_ADDR

# 3. 创建 GPUInferenceAutoscaler 实例（参考 deploy/operator/example-gia.yaml）
kubectl apply -f deploy/operator/example-gia.yaml
```

### 目录与组件

- **api/v1alpha1**：CRD 类型（GPUInferenceAutoscaler、MetricSpec、Prediction、ColdStart）。
- **controllers**：Reconcile 循环，拉取指标 → 可选预测 → 冷启动调整 → 写 Deployment 副本数。
- **pkg/autoscaler/fetcher**：Prometheus 即时/范围查询、Redis 队列长度。
- **pkg/autoscaler/predictor**：指数平滑、线性外推。
- **pkg/autoscaler/coldstart**：冷启动副本修正、扩缩稳定窗口。
- **pkg/autoscaler/scaler**：根据多指标与目标计算 desiredReplicas。

### Operator / Controller 深入理解

从控制面看，系统围绕一个核心 CR：`GPUInferenceAutoscaler`（简称 GIA）运转。

- **Operator 部署面**（`deploy/operator/operator.yaml`）：
  - 创建 `gpu-autoscaler` 命名空间、`gpu-inference-autoscaler` ServiceAccount。
  - 赋予对 `gpuinferenceautoscalers`、`gpuinferenceautoscalers/status`、`deployments` 的读写权限，以及 `pods/events` 只读权限。
  - 部署 Operator 容器（默认镜像 `gpu-inference-autoscaler:latest`），启用 `--leader-elect=true`。
  - 通过环境变量注入 `PROMETHEUS_URL`、`REDIS_ADDR` 作为指标后端连接信息。
  - CRD 本体定义在 `deploy/operator/crd.yaml`（`gpuinferenceautoscalers.autoscaling.gpu.k8s.infra`）。

- **Controller 启动面**（`cmd/operator/main.go`）：
  - 注册 `client-go` 原生资源 + GIA Scheme。
  - 注册默认值逻辑（`RegisterDefaults`），补齐 `minReplicas`、`syncPeriodSeconds`、`scaleTargetRef` 等缺省字段。
  - 构建 `GPUInferenceAutoscalerReconciler`，注入 `Fetcher + Predictor + Scaler`。
  - 启动 manager，进入持续 Reconcile 循环。

- **Controller 触发与调谐面**（`controllers/gia_controller.go`）：
  - Watch 对象：`GPUInferenceAutoscaler`。
  - 每轮流程：
    1) 读取 GIA；若对象删除，清理内部 `lastScale` 缓存。
    2) 读取目标 Deployment（`scaleTargetRef`）。
    3) 从 Deployment 得到 `currentReplicas` 写入状态。
    4) 调用 `Scaler.Compute()` 计算 `desiredReplicas`。
    5) 若刚扩容且当前想缩容，进入冷启动稳定窗口，延迟缩容。
    6) 当 `desired != current` 时更新 Deployment 副本。
    7) 回写 GIA `status`（`current/desired/conditions/currentMetrics/lastScaleTime`）。
    8) 按 `syncPeriodSeconds` 周期再次入队（默认 15 秒）。

- **缩放算法面**（`pkg/autoscaler/scaler`）：
  - 对每个 metric 拉当前值（Prometheus 或 Redis）。
  - 单指标计算：`ceil(value / targetPerReplica)`。
  - 多指标聚合：取最大副本需求（容量保守策略）。
  - 约束：最终副本夹在 `[minReplicas, maxReplicas]`。
  - 可选预测：使用历史序列做指数平滑/线性预测，提前扩容。
  - 冷启动修正：支持 warm pool 与 scale-up 后缩容延迟。

- **可靠性行为**：
  - 幂等：`desired == current` 时不写 Deployment，只更新状态。
  - 错误处理：
    - K8s API 错误返回 runtime 重试。
    - 业务计算错误写 Condition（如 `ComputeError`）并按周期重试。
  - 并发控制：leader election 打开，避免多副本 operator 同时生效。

### 从创建一条 GIA YAML 到 Deployment 副本变化（逐秒时序）

下面是典型一轮从“提交 CR”到“副本落地”的观测脚本与时间线。示例假设：
- GIA 名称：`inference-qps`
- 命名空间：`default`
- 目标 Deployment：`my-gpu-inference`
- 同步周期：15s

```bash
# 0) 安装 CRD + Operator（若尚未安装）
make deploy-operator

# 1) 创建 GIA
kubectl apply -f deploy/operator/example-gia.yaml

# 2) 观察 GIA 状态与目标 Deployment 副本
kubectl get gia -n default -w
kubectl get deploy my-gpu-inference -n default -w

# 3) 观察 controller 日志（另开一个终端）
kubectl logs -n gpu-autoscaler deploy/gpu-inference-autoscaler -f

# 4) 查看完整状态详情（含 currentMetrics / conditions）
kubectl get gia inference-qps -n default -o yaml
```

建议按以下时序理解：

- **T+0s**：`kubectl apply` 提交 GIA，API Server 持久化资源。
- **T+1s**：Controller 收到 GIA 事件，进入首次 Reconcile。
- **T+2~4s**：
  - 读取目标 Deployment；
  - 拉取当前指标（Prometheus/Redis）；
  - 计算 `desiredReplicas`，写入 `status.desiredReplicas/currentMetrics`。
- **T+4~6s**：
  - 若 `desired != current`，更新目标 Deployment `spec.replicas`；
  - 记录 `status.lastScaleTime` 与 `Scaling` condition。
- **T+6~20s**：
  - K8s Deployment Controller 开始滚动扩缩；
  - Pod 进入 `Pending -> Running -> Ready`。
- **T+15s（下一轮）**：
  - GIA 按 `syncPeriodSeconds` 再次 Reconcile；
  - 若仍需要扩缩继续调整；若达到目标写 `Ready` condition。

关键观察点：
- `kubectl get gia -o yaml` 中：
  - `status.currentReplicas`
  - `status.desiredReplicas`
  - `status.currentMetrics`
  - `status.conditions[*].reason/message`
- Operator 日志中的关键词：
  - `compute desired replicas`
  - `scaled deployment`
  - `scale-down delayed by stabilization window`

---

## 许可证

MIT
