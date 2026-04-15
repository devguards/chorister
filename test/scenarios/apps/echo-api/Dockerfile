FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download || true
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o echo-api .

FROM scratch
COPY --from=builder /app/echo-api /echo-api
EXPOSE 8080
ENTRYPOINT ["/echo-api"]
