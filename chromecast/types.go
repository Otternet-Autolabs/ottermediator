package chromecast

import (
	"github.com/i574789/ottermediator/config"
)

// PlaybackState represents the current state of a Chromecast device.
type PlaybackState string

const (
	StatePlaying      PlaybackState = "playing"
	StatePaused       PlaybackState = "paused"
	StateBuffering    PlaybackState = "buffering"
	StateIdle         PlaybackState = "idle"
	StateDisconnected PlaybackState = "disconnected"
)

// DeviceStatus is the snapshot broadcast to the frontend.
type DeviceStatus struct {
	ID            string               `json:"id"`
	Name          string               `json:"name"`
	Addr          string               `json:"addr"`
	App           string               `json:"app"`
	AppIcon       string               `json:"app_icon"`
	AppStatus     string               `json:"app_status"`
	ActiveURL     string               `json:"active_url"` // last URL we cast to this device
	Title         string               `json:"title"`
	Subtitle      string               `json:"subtitle"`
	Artwork       string               `json:"artwork"`
	State         PlaybackState        `json:"state"`
	Position      float32              `json:"position"`
	Duration      float32              `json:"duration"`
	Volume        float32              `json:"volume"`
	Muted         bool                 `json:"muted"`
	KeepaliveMode config.KeepaliveMode `json:"keepalive_mode"`
	KeepaliveURL  string               `json:"keepalive_url"`
}

// Broadcaster is implemented by the WebSocket hub.
type Broadcaster interface {
	Broadcast([]byte)
}

// appIcons maps known Chromecast app IDs to icon URLs.
var appIcons = map[string]string{
	"CA5E8412": "https://assets.nflxext.com/us/ffe/siteui/common/icons/nficon2016.ico",
	"233637DE": "https://www.youtube.com/favicon.ico",
	"CC32E753": "https://open.spotifycdn.com/cdn/images/favicon32.b64ecc03.png",
	"2DB7CC8A": "https://music.youtube.com/favicon.ico",
	"B3DCF968": "https://www.twitch.tv/favicon.ico",
	"9AC194DC": "https://www.plex.tv/favicon.ico",
	"C3DE6BC2": "https://www.disneyplus.com/favicon.ico",
	"A9BCCB7C": "https://www.hbomax.com/favicon.ico",
}

func appIconForID(appID string) string {
	if icon, ok := appIcons[appID]; ok {
		return icon
	}
	return ""
}

func mapPlayerState(s string) PlaybackState {
	switch s {
	case "PLAYING":
		return StatePlaying
	case "PAUSED":
		return StatePaused
	case "BUFFERING":
		return StateBuffering
	default:
		return StateIdle
	}
}
