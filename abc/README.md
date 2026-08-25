# Crosstalk ABC transport

Device-independent Audio Booth Connector client for Crosstalk.

The package owns `/ws/signaling`, ICE, SDP offer/answer/renegotiation, the
reliable `control` data channel, protobuf-v2 Hello/Welcome, one local Opus send
track, and authorized remote audio tracks. It never starts capture, playback,
or encoder processes.

## Consume from another module

Pin a commit. No repository-relative `replace` of `proto/gen/go` is required:

```sh
go get github.com/aleksclark/crosstalk/abc@<commit>
```

```go
sess, err := abc.Dial(ctx, abc.Config{
    ServerURL:      "https://crosstalk.example:8443",
    Token:          os.Getenv("CROSSTALK_ABC_TOKEN"),
    ClientName:     "qol",
    RequireWelcome: true,
})
if err != nil {
    if abc.IsAuthError(err) {
        return err
    }
    return err
}
defer sess.Close()

welcome := sess.Welcome()
codec, ok := sess.NegotiatedCodec()

_ = sess.SendTrack().WriteRTP(pkt)

sess.OnTrack(func(track abc.IncomingTrack) {
    pkt, err := track.ReadRTP()
    _ = pkt
    _ = err
})

<-sess.Done()
```

`Hello.capabilities` advertise the Opus offer (`opus/48000/1` and
`opus/48000/2`). Those fields describe what the client offers; they do not
select the server codec. Use `NegotiatedCodec()` for the SDP-selected
`RTPCodecParameters`.

## Local Crosstalk development

In this repository, `cli/go.mod` and `server/go.mod` may `replace` this module
with `../abc`. That replace is only for in-tree development.
External consumers should pin a tagged or pseudo-versioned commit.

Regenerate the vendored v2 control codec after proto changes:

```sh
task generate:proto:v2
```

That regenerates `abc/internal/controlv2` from `proto/crosstalk/v2/control.proto`
with proto package `abc.internal.controlv2` so it can link beside
`server/proto/v2`. Do not hand-edit field numbers.
