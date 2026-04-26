FROM golang:1.25-bookworm

WORKDIR /app

# Copy go modules for caching.
COPY test/ring/ ./test/ring/
COPY proto/gen/go/ ./proto/gen/go/

RUN cd test/ring && go mod download

# Copy source.
COPY test/ring/ ./test/ring/

WORKDIR /app/test/ring
RUN go build -o /app/bin/ring-test .

CMD ["/app/bin/ring-test"]
