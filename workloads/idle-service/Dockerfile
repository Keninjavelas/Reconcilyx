FROM golang:1.23 as builder
WORKDIR /app
COPY main.go .
RUN CGO_ENABLED=0 GOOS=linux go build -o idle-service main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/idle-service .
EXPOSE 8080
ENTRYPOINT ["./idle-service"]
