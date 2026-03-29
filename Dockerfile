FROM golang:1.26-alpine AS builder

ENV GOTOOLCHAIN=auto
ENV CGO_ENABLED=0

WORKDIR /src

# ❗ СНАЧАЛА копируем ВСЁ, включая staging
COPY kubernetes ./kubernetes
COPY kube-source-explorer ./kube-source-explorer

WORKDIR /src/kube-source-explorer

# Теперь зависимости найдутся
RUN go mod download

# Сборка
RUN go build -o /demo-app .

# Runtime
FROM alpine:3.19
RUN apk --no-cache add ca-certificates

COPY --from=builder /demo-app /app

CMD ["/app"]