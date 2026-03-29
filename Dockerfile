FROM golang:1.22-alpine AS builder
WORKDIR /src

# Копируем и приложение, и нужную часть исходников K8s
COPY ./my-app ./my-app
COPY ./kubernetes/staging ./kubernetes/staging

WORKDIR /src/my-app
RUN go mod download
RUN go build -o demo-app main.go

FROM alpine:latest
COPY --from=builder /src/my-app/demo-app /app
CMD ["/app"]
