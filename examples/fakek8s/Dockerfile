FROM golang:1.23 as builder
WORKDIR /app
COPY go.mod .
RUN go mod download
COPY . .
RUN go build -o /app/tcp-client main.go

FROM debian:bookworm-slim
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]