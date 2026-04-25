package pion

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"sync"
	"syscall"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

// startCapture launches ffmpeg to capture audio from a PipeWire/PulseAudio
// source, encode it as Opus, and stream RTP to a local UDP port. A goroutine
// reads the RTP packets and writes them to the WebRTC track.
//
// sourceName is the PulseAudio device name (e.g.
// "alsa_input.platform-snd_aloop.0.analog-stereo"). If empty, ffmpeg uses the
// default PulseAudio source.
//
// Returns a stop function that kills ffmpeg and closes the UDP listener.
// The stop function is safe to call multiple times.
func startCapture(ctx context.Context, sourceName string, track *webrtc.TrackLocalStaticRTP) (stop func(), err error) {
	if track == nil {
		return nil, fmt.Errorf("track is nil")
	}

	// Bind a free UDP port for ffmpeg to send RTP to.
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen UDP: %w", err)
	}
	localAddr := conn.LocalAddr().String()

	// Build ffmpeg command:
	//   -f pulse -i <source>  → capture from PipeWire/PulseAudio
	//   -c:a libopus …        → Opus encoding
	//   -f rtp rtp://…        → output as RTP
	input := "default"
	if sourceName != "" {
		input = sourceName
	}

	args := []string{
		"-hide_banner", "-loglevel", "warning",
		"-f", "pulse", "-i", input,
		"-c:a", "libopus",
		"-ar", "48000", "-ac", "2",
		"-b:a", "64k",
		"-application", "audio",
		"-frame_duration", "20",
		"-vbr", "on",
		"-f", "rtp", "rtp://" + localAddr,
	}

	captureCtx, captureCancel := context.WithCancel(ctx)

	cmd := exec.CommandContext(captureCtx, "ffmpeg", args...)
	cmd.Stderr = &logWriter{prefix: "ffmpeg-capture"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	slog.Info("capture: starting ffmpeg",
		"source", input,
		"rtp_addr", localAddr,
	)

	if err := cmd.Start(); err != nil {
		captureCancel()
		conn.Close()
		return nil, fmt.Errorf("start ffmpeg: %w", err)
	}

	var stopOnce sync.Once
	stopFn := func() {
		stopOnce.Do(func() {
			captureCancel()
			conn.Close()
			_ = cmd.Wait()
		})
	}

	// Reap ffmpeg in the background so we don't leak zombies.
	go func() {
		if err := cmd.Wait(); err != nil {
			if captureCtx.Err() == nil {
				slog.Error("capture: ffmpeg exited unexpectedly",
					"source", input,
					"err", err,
				)
			}
		} else {
			slog.Info("capture: ffmpeg exited cleanly", "source", input)
		}
	}()

	// Read RTP packets from UDP and forward to the WebRTC track.
	go func() {
		buf := make([]byte, 1500)
		var pkt rtp.Packet
		var pktCount uint64

		for {
			n, _, readErr := conn.ReadFrom(buf)
			if readErr != nil {
				// conn.Close() in stopFn triggers this path — expected.
				if captureCtx.Err() != nil {
					return
				}
				slog.Debug("capture: UDP read error", "err", readErr)
				return
			}

			if err := pkt.Unmarshal(buf[:n]); err != nil {
				slog.Debug("capture: RTP unmarshal error", "err", err)
				continue
			}

			if err := track.WriteRTP(&pkt); err != nil {
				slog.Debug("capture: track WriteRTP error", "err", err)
				return
			}

			pktCount++
			if pktCount == 1 {
				slog.Info("capture: first RTP packet sent to track",
					"source", input,
					"bytes", n,
					"pt", pkt.PayloadType,
					"ssrc", pkt.SSRC,
				)
			} else if pktCount == 50 {
				slog.Info("capture: RTP flowing",
					"source", input,
					"packets", pktCount,
				)
			}
		}
	}()

	slog.Info("capture: audio capture started",
		"source", input,
		"rtp_addr", localAddr,
		"pid", cmd.Process.Pid,
	)

	return stopFn, nil
}
