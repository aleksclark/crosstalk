package pion

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

const (
	opusSampleRate = 48000
	opusChannels   = 1
	opusFrameMs    = 20
	opusFrameSize  = opusSampleRate * opusFrameMs / 1000
	pcmFrameBytes  = opusFrameSize * opusChannels * 2
)

// CaptureSource captures PCM audio from a PipeWire source via pw-record,
// encodes to Opus using ffmpeg, and writes RTP packets to the WebRTC track.
func CaptureSource(ctx context.Context, sourceName string, track *webrtc.TrackLocalStaticRTP) error {
	if track == nil {
		return fmt.Errorf("track is nil")
	}

	// Capture raw PCM from the audio source. Use arecord for ALSA hw: devices,
	// pw-record for PipeWire nodes.
	var pwCmd *exec.Cmd
	if strings.HasPrefix(sourceName, "hw:") {
		pwCmd = exec.CommandContext(ctx, "arecord",
			"-D", sourceName,
			"-f", "S16_LE", "-r", "48000", "-c", "1",
			"-t", "raw", "-")
	} else {
		pwArgs := []string{
			"--format=s16",
			"--rate=48000",
			"--channels=1",
		}
		if sourceName != "" {
			pwArgs = append(pwArgs, "--target="+sourceName)
		}
		pwArgs = append(pwArgs, "-")
		pwCmd = exec.CommandContext(ctx, "pw-record", pwArgs...)
	}
	pwCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	pcmPipe, err := pwCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("pw-record stdout: %w", err)
	}
	if err := pwCmd.Start(); err != nil {
		return fmt.Errorf("starting pw-record: %w", err)
	}
	defer pwCmd.Process.Kill() //nolint:errcheck

	slog.Info("audio capture started", "source", sourceName)

	// Read 20ms PCM frames, encode to Opus via ffmpeg, write RTP.
	// We pipe PCM into ffmpeg, which outputs raw Opus packets.
	// ffmpeg -f s16le -ar 48000 -ac 1 -i pipe:0 -c:a libopus -f rtp rtp://127.0.0.1:<port>
	// We listen on a local UDP port to receive the RTP packets from ffmpeg.

	listener, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen UDP: %w", err)
	}
	defer listener.Close()
	localAddr := listener.LocalAddr().String()

	ffArgs := []string{
		"-hide_banner", "-loglevel", "info",
		"-f", "s16le", "-ar", "48000", "-ac", "1",
		"-i", "pipe:0",
		"-c:a", "libopus",
		"-application", "audio",
		"-frame_duration", "20",
		"-vbr", "on",
		"-b:a", "64k",
		"-f", "rtp", "rtp://" + localAddr,
	}

	ffCmd := exec.CommandContext(ctx, "ffmpeg", ffArgs...)
	ffStdin, err := ffCmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg stdin: %w", err)
	}
	ffCmd.Stderr = &logWriter{prefix: "ffmpeg-encode"}
	ffCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := ffCmd.Start(); err != nil {
		return fmt.Errorf("starting ffmpeg encoder: %w", err)
	}
	defer ffCmd.Process.Kill() //nolint:errcheck

	go func() {
		if waitErr := ffCmd.Wait(); waitErr != nil {
			slog.Error("audio capture: ffmpeg encoder exited", "source", sourceName, "err", waitErr)
		} else {
			slog.Info("audio capture: ffmpeg encoder exited cleanly", "source", sourceName)
		}
	}()

	// Goroutine: pipe pw-record PCM → ffmpeg stdin
	go func() {
		defer ffStdin.Close()
		buf := make([]byte, pcmFrameBytes)
		for {
			n, err := pcmPipe.Read(buf)
			if err != nil {
				return
			}
			if _, err := ffStdin.Write(buf[:n]); err != nil {
				return
			}
		}
	}()

	// Main loop: read RTP from ffmpeg's UDP output → forward to WebRTC track
	rtpBuf := make([]byte, 1500)
	var pktCount uint64
	var timeoutCount int
	for {
		if ctx.Err() != nil {
			return nil
		}
		listener.SetReadDeadline(time.Now().Add(2 * time.Second)) //nolint:errcheck
		n, _, err := listener.ReadFrom(rtpBuf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				timeoutCount++
				if timeoutCount == 1 {
					slog.Warn("audio capture: no RTP from ffmpeg yet (2s timeout)", "source", sourceName)
				} else if timeoutCount%5 == 0 {
					slog.Warn("audio capture: still no RTP from ffmpeg",
						"source", sourceName, "timeouts", timeoutCount, "packets_so_far", pktCount)
				}
				continue
			}
			return fmt.Errorf("reading RTP from ffmpeg: %w", err)
		}
		timeoutCount = 0

		var pkt rtp.Packet
		if err := pkt.Unmarshal(rtpBuf[:n]); err != nil {
			slog.Debug("audio capture: invalid RTP packet", "bytes", n, "err", err)
			continue
		}
		if err := track.WriteRTP(&pkt); err != nil {
			slog.Warn("audio capture: WriteRTP failed", "err", err)
		}

		pktCount++
		if pktCount == 1 {
			slog.Info("audio capture: first RTP packet sent to track",
				"source", sourceName, "bytes", n, "pt", pkt.PayloadType, "ssrc", pkt.SSRC)
		} else if pktCount == 50 {
			slog.Info("audio capture: RTP flowing", "source", sourceName, "packets", pktCount)
		}
	}
}

// PlaybackSink reads RTP packets from a remote WebRTC track, decodes Opus
// using ffmpeg, and pipes PCM audio to a PipeWire sink via pw-cat.
func PlaybackSink(ctx context.Context, sinkName string, remoteTrack *webrtc.TrackRemote) error {
	// Pick a free UDP port for ffmpeg to listen on. We bind temporarily to
	// get an ephemeral port, then close so ffmpeg can bind it.
	tmp, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("pick UDP port: %w", err)
	}
	localAddr := tmp.LocalAddr().String()
	_, port, _ := net.SplitHostPort(localAddr)
	tmp.Close()

	// Write SDP for ffmpeg's RTP demuxer
	sdp := fmt.Sprintf("v=0\no=- 0 0 IN IP4 127.0.0.1\ns=-\nc=IN IP4 127.0.0.1\nt=0 0\nm=audio %s RTP/AVP 111\na=rtpmap:111 opus/48000/2\n", port)

	// pw-cat reads raw PCM from stdin and plays to the named sink.
	pwArgs := []string{
		"-p",
		"--format=s16",
		"--rate=48000",
		"--channels=1",
	}
	if sinkName != "" {
		pwArgs = append(pwArgs, "--target="+sinkName)
	}
	pwArgs = append(pwArgs, "-") // read from stdin

	pwCmd := exec.CommandContext(ctx, "pw-cat", pwArgs...)
	pcmPipe, err := pwCmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("pw-cat stdin: %w", err)
	}
	if err := pwCmd.Start(); err != nil {
		return fmt.Errorf("starting pw-cat: %w", err)
	}
	defer pwCmd.Process.Kill() //nolint:errcheck
	defer pcmPipe.Close()      //nolint:errcheck

	// ffmpeg decodes Opus RTP from the local UDP port → outputs raw PCM to stdout.
	ffArgs := []string{
		"-hide_banner", "-loglevel", "info",
		"-protocol_whitelist", "pipe,file,udp,rtp",
		"-f", "sdp", "-i", "pipe:0",
		"-f", "s16le", "-ar", "48000", "-ac", "1",
		"pipe:1",
	}

	ffCmd := exec.CommandContext(ctx, "ffmpeg", ffArgs...)
	ffStdin, err := ffCmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg stdin: %w", err)
	}
	ffStdout, err := ffCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg stdout: %w", err)
	}
	ffCmd.Stderr = &logWriter{prefix: "ffmpeg-decode"}
	if err := ffCmd.Start(); err != nil {
		return fmt.Errorf("starting ffmpeg decoder: %w", err)
	}
	defer ffCmd.Process.Kill() //nolint:errcheck

	// Write the SDP to ffmpeg's stdin so it knows the RTP format
	if _, err := ffStdin.Write([]byte(sdp)); err != nil {
		return fmt.Errorf("writing SDP to ffmpeg: %w", err)
	}
	ffStdin.Close() //nolint:errcheck

	slog.Info("audio playback started", "sink", sinkName, "rtp_port", port)

	go func() {
		if waitErr := ffCmd.Wait(); waitErr != nil {
			slog.Error("audio playback: ffmpeg decoder exited", "sink", sinkName, "err", waitErr)
		}
	}()

	// Goroutine: pipe ffmpeg PCM stdout → pw-cat stdin
	go func() {
		io.Copy(pcmPipe, ffStdout) //nolint:errcheck
		pcmPipe.Close()            //nolint:errcheck
	}()

	// Give ffmpeg time to parse SDP and bind the UDP port.
	time.Sleep(1 * time.Second)

	// Resolve the local address for writing RTP packets
	udpAddr, err := net.ResolveUDPAddr("udp", localAddr)
	if err != nil {
		return fmt.Errorf("resolve UDP addr: %w", err)
	}
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return fmt.Errorf("dial UDP: %w", err)
	}
	defer conn.Close()

	// Main loop: read RTP from WebRTC track → write to local UDP for ffmpeg
	ssrc := rand.Uint32()
	_ = ssrc
	for {
		if ctx.Err() != nil {
			return nil
		}
		remoteTrack.SetReadDeadline(time.Now().Add(5 * time.Second)) //nolint:errcheck
		pkt, _, err := remoteTrack.ReadRTP()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("reading RTP: %w", err)
		}

		raw, err := pkt.Marshal()
		if err != nil {
			continue
		}
		if _, err := conn.Write(raw); err != nil {
			slog.Debug("udp write error", "err", err)
		}
	}
}

// logWriter adapts subprocess stderr to slog.
type logWriter struct {
	prefix string
	buf    []byte
}

func (w *logWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := 0
		for i < len(w.buf) && w.buf[i] != '\n' {
			i++
		}
		if i >= len(w.buf) {
			break
		}
		line := string(w.buf[:i])
		w.buf = w.buf[i+1:]
		if len(line) > 0 {
			slog.Warn("subprocess stderr", "proc", w.prefix, "line", line)
		}
	}
	return len(p), nil
}
