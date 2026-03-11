# GPU Inference Autoscaler Operator
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /operator ./cmd/operator

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /operator /operator
EXPOSE 8081
ENTRYPOINT ["/operator"]
