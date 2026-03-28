# OtterMediator

A local media cockpit for monitoring and controlling Chromecast devices on your network.

## Features

- **Auto-discovery** — finds Chromecast devices via mDNS automatically
- **Live status** — real-time artwork, title, playback state, position, and volume via WebSocket
- **Playback controls** — play/pause, stop, seek, next/prev, volume, mute
- **Cast URL** — send any URL to a Chromecast
- **Keep-alive modes** per device:
  - **Off** — no automation
  - **Keep-alive** — relaunch assigned URL if the device goes idle on its own; backs off if someone else takes over
  - **Force** — always maintain the assigned URL regardless of interruption
- **Persistent config** — device assignments survive restarts

## Running

```bash
go build -o ottermediator .
./ottermediator
```

Open `http://localhost:8006`

### Options

```
--port 8006          Port to listen on (also reads PORT env var)
--config path.json   Config file path (default: ottermediator.json)
```

## Extending

The architecture is designed for future device types (Roku, Apple TV). The `Broadcaster` interface and `DiscoveryManager` pattern can be replicated for additional platforms.

## Stack

- Go backend — single binary, embeds frontend
- `go-chromecast` for Chromecast protocol
- `gorilla/websocket` for live updates
- Vanilla JS frontend — no build step
