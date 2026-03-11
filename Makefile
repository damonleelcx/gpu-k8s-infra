.PHONY: build build-webapp build-operator docker run deploy-base deploy-operator

# Build Go API binary
build:
	go build -o bin/api ./cmd/api

# Build GPU Inference Autoscaler operator
build-operator:
	go build -o bin/operator ./cmd/operator

# Build webapp for production (output to dist/)
build-webapp:
	cd webapp && npm ci && npm run build

# Build API Docker image
docker:
	docker build -t gpu-platform-api:latest .

# Build operator Docker image
docker-operator:
	docker build -f cmd/operator/Dockerfile -t gpu-inference-autoscaler:latest .

# Run API locally (needs KUBECONFIG or in-cluster)
run:
	go run ./cmd/api

# Run operator locally (needs KUBECONFIG, optional PROMETHEUS_URL / REDIS_ADDR)
run-operator:
	go run ./cmd/operator --leader-elect=false

# Apply base K8s manifests (namespace + API + HPA)
deploy-base:
	kubectl apply -f deploy/base/namespace.yaml
	kubectl apply -f deploy/base/api-deployment.yaml
	kubectl apply -f deploy/base/api-hpa.yaml

# Install CRD + GPU Inference Autoscaler operator
deploy-operator:
	kubectl apply -f deploy/operator/crd.yaml
	kubectl apply -f deploy/operator/operator.yaml

# Install metrics-server (for HPA and kubectl top)
install-metrics-server:
	kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml

# Tidy Go modules
tidy:
	go mod tidy
