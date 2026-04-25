package pion

import (
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/pion/webrtc/v4"
)

// ForwardTrack sets up RTP forwarding from the source peer's incoming audio
// track to the sink peer via a TrackLocalStaticRTP. It returns a stop function
// that terminates the forwarding goroutine.
//
// Track manipulation (AddTrack/RemoveTrack) is performed directly on the raw
// Pion PeerConnection rather than through a PeerConn-level API. This is
// intentional: the SFU forwarding logic needs fine-grained control over track
// senders and receivers, and an abstraction layer would add indirection without
// meaningful safety benefits. All track lifecycle management is co-located here.
//
// The implementation:
//  1. Creates a TrackLocalStaticRTP on the sink side for Opus audio.
//  2. Adds the local track to the sink's PeerConnection.
//  3. Registers an OnTrack handler on the source peer — when the matching
//     track arrives (identified by trackLabel in the stream ID), it reads
//     RTP packets and writes them to the local track.
//  4. Returns a stop function that closes the forwarding goroutine.
func ForwardTrack(sourcePeer, sinkPeer *PeerConn, trackLabel string) (stop func(), err error) {
	// Create a local track on the sink side. We use Opus/48000 as the default
	// audio codec. The stream ID is the trackLabel so we can correlate.
	localTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		trackLabel,    // track ID
		trackLabel,    // stream ID
	)
	if err != nil {
		return nil, fmt.Errorf("forward: create local track: %w", err)
	}

	// Add the local track to the sink's PeerConnection. This creates a sender
	// that will transmit the forwarded packets to the sink client.
	slog.Info("forward: adding local track to sink peer",
		"track", trackLabel,
		"source_peer", sourcePeer.ID,
		"sink_peer", sinkPeer.ID)

	sender, err := sinkPeer.pc.AddTrack(localTrack)
	if err != nil {
		return nil, fmt.Errorf("forward: add track to sink: %w", err)
	}

	slog.Info("forward: track added to sink peer, triggering renegotiation",
		"track", trackLabel,
		"sink_peer", sinkPeer.ID)

	// Trigger renegotiation so the sink client learns about the new track.
	sinkPeer.Negotiate()

	var (
		stopOnce sync.Once
		done     = make(chan struct{})
	)

	stopFn := func() {
		stopOnce.Do(func() {
			close(done)
			// Remove the track from the sink's peer connection.
			if removeErr := sinkPeer.pc.RemoveTrack(sender); removeErr != nil {
				slog.Debug("forward: remove track from sink",
					"track", trackLabel, "err", removeErr)
			}
		})
	}

	// Read and discard RTCP packets from the sink's sender to avoid blocking.
	go func() {
		buf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := sender.Read(buf); rtcpErr != nil {
				return
			}
		}
	}()

	// Register OnTrack on the source peer to capture the incoming audio track.
	// When the source client adds a track matching our stream/track ID, we
	// start forwarding RTP packets to the local track.
	sourcePeer.pc.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		slog.Debug("forward: received remote track",
			"track", trackLabel,
			"remote_id", remoteTrack.ID(),
			"remote_stream", remoteTrack.StreamID(),
			"codec", remoteTrack.Codec().MimeType)

		// Forward RTP packets from the remote track to the local track.
		go func() {
			buf := make([]byte, 1500)
			for {
				select {
				case <-done:
					return
				default:
				}

				n, _, readErr := remoteTrack.Read(buf)
				if readErr != nil {
					if readErr == io.EOF {
						slog.Debug("forward: remote track EOF", "track", trackLabel)
					}
					return
				}

				if _, writeErr := localTrack.Write(buf[:n]); writeErr != nil {
					if writeErr == io.ErrClosedPipe {
						return
					}
					slog.Debug("forward: write to local track",
						"track", trackLabel, "err", writeErr)
				}
			}
		}()
	})

	return stopFn, nil
}
