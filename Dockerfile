FROM golang:1.23.3-alpine3.20
WORKDIR /src
RUN apk --no-cache add git
COPY main.go go.mod go.sum ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/prometheus-msk-discovery .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=0 /bin/prometheus-msk-discovery /bin/
ENTRYPOINT ["prometheus-msk-discovery"]
