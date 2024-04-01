FROM golang:1.22.1
ARG BUILD_VERSION
WORKDIR /weeth
COPY . .

RUN go mod download && go mod verify
RUN go build -o /usr/local/bin/app ./cmd/main.go

FROM debian:bookworm-slim
COPY --from=0 /usr/local/bin/app /usr/local/bin/app
RUN apt-get update && apt-get install -y ca-certificates

CMD ["app"]
