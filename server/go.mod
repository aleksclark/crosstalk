module github.com/aleksclark/crosstalk/server

go 1.25.5

require (
	github.com/aleksclark/crosstalk/proto/gen/go v0.0.0-00010101000000-000000000000
	github.com/go-chi/chi/v5 v5.2.5
	github.com/mattn/go-sqlite3 v1.14.42
	github.com/oklog/ulid/v2 v2.1.1
	github.com/pion/ice/v4 v4.2.2
	github.com/pion/rtp v1.10.1
	github.com/pion/webrtc/v4 v4.2.11
	github.com/pressly/goose/v3 v3.27.0
	github.com/stretchr/testify v1.11.1
	golang.org/x/crypto v0.48.0
	google.golang.org/protobuf v1.36.11
	nhooyr.io/websocket v1.8.17
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mfridman/interpolate v0.0.2 // indirect
	github.com/pion/datachannel v1.6.0 // indirect
	github.com/pion/dtls/v3 v3.1.2 // indirect
	github.com/pion/interceptor v0.1.44 // indirect
	github.com/pion/logging v0.2.4 // indirect
	github.com/pion/mdns/v2 v2.1.0 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/rtcp v1.2.16 // indirect
	github.com/pion/sctp v1.9.4 // indirect
	github.com/pion/sdp/v3 v3.0.18 // indirect
	github.com/pion/srtp/v3 v3.0.10 // indirect
	github.com/pion/stun/v3 v3.1.1 // indirect
	github.com/pion/transport/v4 v4.0.1 // indirect
	github.com/pion/turn/v4 v4.1.4 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/sethvargo/go-retry v0.3.0 // indirect
	github.com/wlynxg/anet v0.0.5 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/net v0.50.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/time v0.10.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/aleksclark/crosstalk/proto/gen/go => ../proto/gen/go
