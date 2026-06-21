FROM golang:1.23 as builder
WORKDIR /app
COPY main.go .
RUN CGO_ENABLED=0 GOOS=linux go build -o memory-leaker main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/memory-leaker .
EXPOSE 8080
ENTRYPOINT ["./memory-leaker"]
