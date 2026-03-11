# Build stage
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /api ./cmd/api

# Run stage
FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /api /api
EXPOSE 8080
ENTRYPOINT ["/api"]
