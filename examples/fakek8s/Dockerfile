FROM registry.suse.com/bci/golang:1.27@sha256:182c55128c46dd2284ce07a9afaa4c05307c92f8014e0d3b0c256a4643b01923 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:16.1@sha256:fefab9e733eb35ae19294ed3db2a408794f297d765c472199a1cac0ec00f39af
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
