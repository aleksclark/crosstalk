# CrossTalk

Realtime audio/video/data bridge/muxing system using webrtc

Supports arbitrary interconnection of webrtc channels among connected clients. Primary use-case is:

clientA_input -> clientB_output
clientA_input -> server_record
clientB_input -> server_record
clientB_input -> broadcast to N clients

Initially focused on audio streaming, later expandable to connect video channels


## Technical design

### Server/CLI

use https://medium.com/@benbjohnson/standard-package-layout-7cdbc8391fc1 for project layout
use pion for webrtc
latest go, manage with asdf
testify to ensure test coverage
follow go doc best practices

### Server <-> Client

* REST API, generate openapi spec by deriving types, NOT manual
* webrtc data channel for streaming data
* protobuf schema for channel messages -> generated go + typescript types
* typescript client library generated from openapi spec

### Web UI
latest node/typescript, manage with asdf + pnpm
react + vite + typescript (strict) + shadcn dark mode
ensure test coverage

## Components

### Server
written in go
sqlite + goose for persistance
REST API auth via token or username/password
Webrtc client auth via token generated on request from REST API - 24 hour lifetime
tracks connected webrtc clients, channels, sources/sinks

### CLI Client
auth to REST API via token in env var, establish webrtc connection
dynamically reports sources/sinks to server - initially we want to just support pipewire sources/sinks. Should support naming sources/sinks via env var
dynamically reports channel codec support to server - use standard webrtc formatting
connects webrtc channels to sources/sinks on command from server, reports connection status to server

### Admin Web
login
server status dashboard
manage REST API tokens, users
manage sessions + session templates
display connected clients, sources, codecs

#### Session Connect
Admin ui allows connecting to a session as a client - browser mic as source, browser audio as sink
In this view there should be a VU meter + volume control for each audio channel coming in.
There should be a pane with webrtc debug information - all the stuff about ICE, STUN, channels, etc from the browser itself
There should be another pane with server logs about the session - all signaling/status messages streamed over the webrtc data channel, for all clients connected to the session.
Rapid iteration and testing is critical - the dashboard should have a button that creates a new session from the default session template and connects to it in the "translator" role

## Data Model
These are more guidelines than anything:

### Channels
the channel has a type (audio, video, logstream) and a direction (source/sink), and is also named.
clients establish a special channel (we'll call it `control`) for live communication with the server. This will NOT be used in sessions
the logstream channel will be defined in protobuf, supporting timestamp, severity, and message fields

### Session Template
consists of channel mappings between roles. 
eg a template called "Translation":

roles: translator, studio, audience 
mappings:
* translator:mic -> studio:output
* studio:input -> translator:speakers
* translator:mic -> audience:speakers
* translator:mic -> record
* studio:input -> record


### Session
Clients should connect to the session as a particular role, server should track status of connections for the session

## Hardware

The CLI client will primarily be run on a kickpi k2b board, capturing and sending audio via the TRRS jack

## Dev Environment
1. web ui running in a docker container, volume mounting the source, with hot reload, macvlan networking
2. server running in a docker container, volume mounting the binary + sqlite file, hot reload/restart, macvlan networking
3. cli client auto-deploying to k2b device via a watcher script. create a script to play an mp3 into a pipewire loopback that the cli client uses as a source, while recording an mp3 from the sink

## Testing

Overall philosophy: agent-driven development has a critical pitfall of saying stuff is done when it's not. Our testing environment must be iron-clad, demonstrating actual functionality and not mocks or stubs. We may use mocks/stubs at the unit test level for speed, but that's it.

1. unit tests with good coverage
2. integration tests run in separate docker compose environment, use playwright for the web ui, ensure setup/teardown is clean and reproducible
3. e2e tests run in the dev environment - golden tests:
  * sending actual audio from the admin web ui -> k2b device, inspecting recorded audio captured from pipewire loopback
  * sending audio from k2b device via pipewire loopback -> admin web ui, inspecting recorded audio captured via playwright
