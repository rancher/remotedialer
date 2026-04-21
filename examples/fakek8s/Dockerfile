FROM registry.suse.com/bci/golang:1.25@sha256:3190609b31bbdbd4396ecba6c17ef71ccd282ec6973dba7c106b391b3edb1807 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:15.7@sha256:590f1bead5a54cfd3cee7b7d4459f874f3e1b2789a7d2149a2d80b35a43ac9cf
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
