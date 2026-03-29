# Используем свежий образ, так как в staging уже 1.26
FROM golang:1.24-alpine AS builder

# Позволяем скачивать тулчейн 1.26 автоматически
ENV GOTOOLCHAIN=auto
ENV CGO_ENABLED=0

WORKDIR /src

# 1. Копируем обе папки (запуск сборки из родительской директории!)
COPY ./kubernetes ./kubernetes
COPY ./kube-source-explorer ./kube-source-explorer

# 2. Переходим в папку проекта
WORKDIR /src/kube-source-explorer

# 3. Скачиваем зависимости (теперь ../kubernetes/staging доступен)
RUN go mod download

# 4. Собираем бинарник из НОВОГО пути (cmd/explorer)
RUN go build -o /demo-app ./cmd/explorer/main.go

# Runtime слой
FROM alpine:3.19
RUN apk --no-cache add ca-certificates

COPY --from=builder /demo-app /app

# По умолчанию запускаем статус, но можно переопределить через args в YAML
CMD ["/app", "status"]
