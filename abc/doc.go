// Package abc is a device-independent Crosstalk Audio Booth Connector
// transport. It owns signaling, protobuf-v2 control, SDP/ICE, one local
// send track, and authorized remote audio tracks.
//
// The package never starts capture, playback, or encoder processes.
// Callers feed and consume RTP/encoded frames through the Session API.
//
// External modules should depend on this package directly:
//
//	go get github.com/aleksclark/crosstalk/abc@<commit>
//
// No repository-relative replace of proto/gen/go is required. Crosstalk's
// own tree may still use a local replace or go.work while developing.
package abc
