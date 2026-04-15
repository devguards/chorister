FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download || true
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o security-trigger .

FROM alpine:3.19
# Need /bin/sh for the exec-shell test
RUN apk add --no-cache bash
COPY --from=builder /app/security-trigger /security-trigger
EXPOSE 8080
ENTRYPOINT ["/security-trigger"]
