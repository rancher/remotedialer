FROM golang:1.25 as builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM debian:bookworm-slim
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
