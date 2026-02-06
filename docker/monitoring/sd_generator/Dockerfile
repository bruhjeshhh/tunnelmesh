FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY main.go .
RUN go build -o sd-generator main.go

FROM alpine:latest

RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /app/sd-generator .

ENTRYPOINT ["/app/sd-generator"]
