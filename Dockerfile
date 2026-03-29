FROM golang:1.24-alpine AS builder
ENV GOTOOLCHAIN=auto
ENV CGO_ENABLED=0
WORKDIR /src

# --- ЭТАП 1: КЭШИРОВАНИЕ ЗАВИСИМОСТЕЙ ---
# Копируем ТОЛЬКО файлы go.mod, чтобы скачать библиотеки один раз
COPY ./kube-source-explorer/go.mod ./kube-source-explorer/go.sum ./kube-source-explorer/
# Нам нужны go.mod из staging, чтобы локальный replace сработал
COPY ./kubernetes/staging/src/k8s.io/api/go.mod ./kubernetes/staging/src/k8s.io/api/
COPY ./kubernetes/staging/src/k8s.io/apimachinery/go.mod ./kubernetes/staging/src/k8s.io/apimachinery/
COPY ./kubernetes/staging/src/k8s.io/client-go/go.mod ./kubernetes/staging/src/k8s.io/client-go/

WORKDIR /src/kube-source-explorer
RUN go mod download

# --- ЭТАП 2: КОПИРОВАНИЕ КОДА И СБОРКА ---
WORKDIR /src
# Теперь копируем всё остальное. Этот шаг будет срабатывать быстро
COPY ./kubernetes ./kubernetes
COPY ./kube-source-explorer ./kube-source-explorer

WORKDIR /src/kube-source-explorer
RUN go build -o /demo-app ./cmd/explorer/main.go

# Runtime
FROM alpine:3.19
RUN apk --no-cache add ca-certificates
COPY --from=builder /demo-app /app
CMD ["/app", "status"]
