# ==============================
# Builder stage (сборка)
# ==============================
FROM golang:1.26-alpine AS builder

# Включаем современный кэш компиляции Go
ENV CGO_ENABLED=0
WORKDIR /src

# 1. Сначала копируем ТОЛЬКО файлы описания зависимостей
COPY ./kube-source-explorer/go.mod ./kube-source-explorer/go.sum ./kube-source-explorer/
COPY ./kubernetes/staging/src/k8s.io/api/go.mod ./kubernetes/staging/src/k8s.io/api/
COPY ./kubernetes/staging/src/k8s.io/apimachinery/go.mod ./kubernetes/staging/src/k8s.io/apimachinery/
COPY ./kubernetes/staging/src/k8s.io/client-go/go.mod ./kubernetes/staging/src/k8s.io/client-go/

WORKDIR /src/kube-source-explorer

# 2. Скачиваем библиотеки (теперь это отдельный слой, который закэшируется)
# --mount=type=cache сохранит скачанные модули между сборками
RUN --mount=type=cache,target=/go/pkg/mod go mod download

# 3. Копируем исходный код только после подготовки библиотек
WORKDIR /src
COPY ./kubernetes/staging ./kubernetes/staging
COPY ./kube-source-explorer ./kube-source-explorer

WORKDIR /src/kube-source-explorer

# 4. Собираем бинарник с использованием кэша компилятора
# Это позволит пересобирать только меняемые файлы, не трогая либы K8s
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -o /demo-app ./cmd/explorer/main.go

# ==============================
# Runtime stage (исполнение)
# ==============================
FROM alpine:3.19

RUN apk --no-cache add ca-certificates

# Копируем готовый файл из билдера
COPY --from=builder /demo-app /app

# Твоя точка входа
CMD ["/app", "status"]
