# ==============================
# Builder stage (сборка)
# ==============================
FROM golang:1.26-alpine AS builder

# Отключаем CGO, чтобы получить статически линкованный бинарник
ENV CGO_ENABLED=0
ENV GOTOOLCHAIN=auto

# Устанавливаем рабочую директорию
WORKDIR /src

# --- ЭТАП 1: КЭШИРОВАНИЕ ЗАВИСИМОСТЕЙ ---
# Копируем только go.mod/go.sum, чтобы использовать кэш зависимостей
COPY ./kube-source-explorer/go.mod ./kube-source-explorer/go.sum ./kube-source-explorer/

# Kubernetes staging модули, чтобы локальные replace в go.mod работали
COPY ./kubernetes/staging/src/k8s.io/api/go.mod ./kubernetes/staging/src/k8s.io/api/
COPY ./kubernetes/staging/src/k8s.io/apimachinery/go.mod ./kubernetes/staging/src/k8s.io/apimachinery/
COPY ./kubernetes/staging/src/k8s.io/client-go/go.mod ./kubernetes/staging/src/k8s.io/client-go/

WORKDIR /src/kube-source-explorer

# Скачиваем зависимости (не копируя весь код — кэш будет использоваться)
RUN go mod download

# --- ЭТАП 2: КОПИРОВАНИЕ КОДА И СБОРКА ---
WORKDIR /src

# Копируем все исходники
COPY ./kubernetes ./kubernetes
COPY ./kube-source-explorer ./kube-source-explorer

WORKDIR /src/kube-source-explorer

# Сборка бинарника
RUN go build -o /demo-app ./cmd/explorer/main.go

# ==============================
# Runtime stage (исполнение)
# ==============================
FROM alpine:3.19

# Нужно для HTTPS и CA
RUN apk --no-cache add ca-certificates

# Копируем собранный бинарник
COPY --from=builder /demo-app /app

# Точка входа
CMD ["/app", "status"]