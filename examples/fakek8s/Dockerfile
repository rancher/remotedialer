FROM registry.suse.com/bci/golang:1.27@sha256:107c1d9c1be31452eac6d71afff3fc33144001e515f4f7e843ebbdeedc5cee12 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:16.1@sha256:8e774758e9a9b081b777f4290a0779bdfe9d44f662793dd9fd1c5e79c8c5a156
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
