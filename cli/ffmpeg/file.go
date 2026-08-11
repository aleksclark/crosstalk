// Package ffmpeg decodes audio files to Opus RTP via the external ffmpeg binary.
package ffmpeg

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/pion/rtp"
)

// FileSource streams a WAV or MP3 file as Opus RTP packets in real time.
type FileSource struct {
	// Binary is the ffmpeg executable name or path (default "ffmpeg").
	Binary string
	// Logger receives structured diagnostics without secrets.
	Logger *slog.Logger
}

// StreamArgs returns the ffmpeg argv used for real-time file → Opus RTP.
// Exported for unit tests that assert pacing and codec parameters.
func StreamArgs(inputPath, rtpURL string) []string {
	return []string{
		"-hide_banner", "-loglevel", "error",
		"-re",
		"-i", inputPath,
		"-vn",
		"-ac", "1",
		"-ar", "48000",
		"-c:a", "libopus",
		"-application", "audio",
		"-frame_duration", "20",
		"-vbr", "on",
		"-b:a", "64k",
		"-f", "rtp", rtpURL,
	}
}

// Stream runs ffmpeg against path and invokes write for each complete RTP packet.
func (s *FileSource) Stream(ctx context.Context, path string, write func(packet []byte) error) error {
	if write == nil {
		return fmt.Errorf("write callback is required")
	}
	bin := s.Binary
	if bin == "" {
		bin = "ffmpeg"
	}
	log := s.Logger
	if log == nil {
		log = slog.Default()
	}

	listener, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen local RTP: %w", err)
	}
	defer listener.Close()
	localAddr := listener.LocalAddr().String()
	rtpURL := "rtp://" + localAddr

	args := StreamArgs(path, rtpURL)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}

	// Capture bounded stderr for error reporting.
	errCh := make(chan string, 1)
	go func() {
		raw, _ := io.ReadAll(io.LimitReader(stderr, 8<<10))
		errCh <- strings.TrimSpace(string(raw))
	}()

	// Ensure process group is reaped.
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	cleanup := func() {
		if cmd.Process != nil {
			// Kill the whole process group.
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			_ = cmd.Process.Kill()
		}
		select {
		case <-waitCh:
		case <-time.After(2 * time.Second):
		}
	}
	defer cleanup()

	buf := make([]byte, 2048)
	var pktCount int
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_ = listener.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, _, rerr := listener.ReadFrom(buf)
		if rerr != nil {
			if ne, ok := rerr.(net.Error); ok && ne.Timeout() {
				select {
				case werr := <-waitCh:
					if ctx.Err() != nil {
						return ctx.Err()
					}
					stderrMsg := ""
					select {
					case stderrMsg = <-errCh:
					default:
					}
					if werr != nil {
						if stderrMsg != "" {
							return fmt.Errorf("ffmpeg exited: %w (%s)", werr, stderrMsg)
						}
						return fmt.Errorf("ffmpeg exited: %w", werr)
					}
					if pktCount == 0 {
						if stderrMsg != "" {
							return fmt.Errorf("ffmpeg produced no RTP packets (%s)", stderrMsg)
						}
						return fmt.Errorf("ffmpeg produced no RTP packets")
					}
					// Clean EOF after packets were written.
					return nil
				default:
					continue
				}
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("read RTP: %w", rerr)
		}
		if n < 12 {
			continue
		}
		packet := make([]byte, n)
		copy(packet, buf[:n])
		// Validate RTP header before forwarding.
		var parsed rtp.Packet
		if err := parsed.Unmarshal(packet); err != nil {
			log.Debug("skip malformed RTP from ffmpeg", "error", err.Error())
			continue
		}
		if err := write(packet); err != nil {
			return err
		}
		pktCount++
	}
}
