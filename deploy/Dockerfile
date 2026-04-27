FROM golang:1.25-bookworm AS build

RUN apt-get update && apt-get install -y --no-install-recommends gcc sqlite3

WORKDIR /app
COPY server/go.mod server/go.sum ./server/
COPY proto/gen/go/go.mod proto/gen/go/go.sum ./proto/gen/go/
RUN cd server && go mod download

COPY server/ ./server/
COPY proto/ ./proto/
COPY web/dist/ ./server/web/dist/

RUN cd server && CGO_ENABLED=1 go build -o /app/ct-server ./cmd/ct-server

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates sqlite3 curl \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /app/ct-server /usr/local/bin/ct-server
COPY deploy/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

EXPOSE 8080
EXPOSE 5000/udp

ENTRYPOINT ["entrypoint.sh"]
