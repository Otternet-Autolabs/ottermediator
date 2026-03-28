package chromecast

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/i574789/ottermediator/config"
	castdns "github.com/vishen/go-chromecast/dns"
	"github.com/vishen/go-chromecast/application"
)

// Device manages the connection and state for a single Chromecast.
type Device struct {
	mu            sync.Mutex
	status        DeviceStatus
	app           *application.Application
	entry         castdns.CastEntry
	cfg           *config.Config
	hub           Broadcaster
	prevState     PlaybackState
	lastCastURL   string
	relaunchTimer *time.Timer
	stopCh        chan struct{}
}

func newDevice(entry castdns.CastEntry, cfg *config.Config, hub Broadcaster) *Device {
	dc := cfg.GetDevice(entry.GetUUID())
	return &Device{
		cfg:    cfg,
		hub:    hub,
		entry:  entry,
		stopCh: make(chan struct{}),
		status: DeviceStatus{
			ID:            entry.GetUUID(),
			Name:          entry.GetName(),
			Addr:          entry.GetAddr(),
			State:         StateDisconnected,
			KeepaliveMode: dc.KeepaliveMode,
			KeepaliveURL:  dc.KeepaliveURL,
		},
	}
}

// Start begins the poll loop for this device.
func (d *Device) Start() {
	go d.connectAndPoll()
}

// Shutdown stops the poll loop for this device.
func (d *Device) Shutdown() {
	close(d.stopCh)
}

// GetStatus returns a copy of the current device status.
func (d *Device) GetStatus() DeviceStatus {
	d.mu.Lock()
	defer d.mu.Unlock()
	s := d.status
	// Merge live keepalive config
	dc := d.cfg.GetDevice(d.status.ID)
	s.KeepaliveMode = dc.KeepaliveMode
	s.KeepaliveURL = dc.KeepaliveURL
	return s
}

func (d *Device) connectAndPoll() {
	backoff := time.Second
	for {
		select {
		case <-d.stopCh:
			return
		default:
		}

		d.mu.Lock()
		app := application.NewApplication(application.WithConnectionRetries(1))
		err := app.Start(d.entry)
		if err != nil {
			d.mu.Unlock()
			log.Printf("[%s] connect failed: %v, retrying in %s", d.status.Name, err, backoff)
			select {
			case <-d.stopCh:
				return
			case <-time.After(backoff):
			}
			if backoff < 60*time.Second {
				backoff *= 2
			}
			continue
		}
		d.app = app
		d.mu.Unlock()
		backoff = time.Second

		log.Printf("[%s] connected", d.status.Name)
		d.runPollLoop()

		// Poll loop exited — device disconnected
		d.mu.Lock()
		if d.app != nil {
			d.app.Close()
			d.app = nil
		}
		d.status.State = StateDisconnected
		d.mu.Unlock()
		d.broadcast()
	}
}

func (d *Device) runPollLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-d.stopCh:
			return
		case <-ticker.C:
			if !d.poll() {
				return // disconnect detected
			}
		}
	}
}

func (d *Device) poll() bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.app == nil {
		return false
	}

	if err := d.app.Update(); err != nil {
		log.Printf("[%s] update error: %v", d.status.Name, err)
		return false
	}

	castApp, castMedia, castVol := d.app.Status()

	prev := d.status
	d.prevState = prev.State

	// Map status fields
	if castApp != nil && !castApp.IsIdleScreen {
		d.status.App = castApp.DisplayName
		d.status.AppIcon = appIconForID(castApp.AppId)
	} else {
		d.status.App = ""
		d.status.AppIcon = ""
	}

	if castMedia != nil {
		d.status.State = mapPlayerState(castMedia.PlayerState)
		d.status.Position = castMedia.CurrentTime
		d.status.Duration = castMedia.Media.Duration
		meta := castMedia.Media.Metadata
		d.status.Title = meta.Title
		d.status.Subtitle = meta.Subtitle
		if d.status.Subtitle == "" {
			d.status.Subtitle = meta.Artist
		}
		if len(meta.Images) > 0 {
			d.status.Artwork = meta.Images[0].URL
		} else {
			d.status.Artwork = ""
		}
	} else {
		d.status.State = StateIdle
		d.status.Position = 0
		d.status.Duration = 0
		d.status.Title = ""
		d.status.Subtitle = ""
		d.status.Artwork = ""
	}

	if castVol != nil {
		d.status.Volume = castVol.Level
		d.status.Muted = castVol.Muted
	}

	// Evaluate keep-alive (while still holding the lock)
	evaluateKeepalive(d)

	// Broadcast outside the lock to avoid deadlock — copy first
	go func(snap DeviceStatus) {
		dc := d.cfg.GetDevice(snap.ID)
		snap.KeepaliveMode = dc.KeepaliveMode
		snap.KeepaliveURL = dc.KeepaliveURL
		msg, _ := json.Marshal(map[string]interface{}{
			"type": "devices",
			"data": []DeviceStatus{snap},
		})
		d.hub.Broadcast(msg)
	}(d.status)

	return true
}

func (d *Device) broadcast() {
	s := d.status
	dc := d.cfg.GetDevice(s.ID)
	s.KeepaliveMode = dc.KeepaliveMode
	s.KeepaliveURL = dc.KeepaliveURL
	msg, _ := json.Marshal(map[string]interface{}{
		"type": "devices",
		"data": []DeviceStatus{s},
	})
	d.hub.Broadcast(msg)
}

// ── Control methods ──────────────────────────────────────────────────────────

func (d *Device) Play() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.app == nil {
		return errDisconnected
	}
	return d.app.Unpause()
}

func (d *Device) Pause() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.app == nil {
		return errDisconnected
	}
	return d.app.Pause()
}

func (d *Device) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.app == nil {
		return errDisconnected
	}
	return d.app.StopMedia()
}

func (d *Device) Seek(position float32) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.app == nil {
		return errDisconnected
	}
	return d.app.SeekToTime(position)
}

func (d *Device) SetVolume(level float32, muted bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.app == nil {
		return errDisconnected
	}
	if err := d.app.SetVolume(level); err != nil {
		return err
	}
	return d.app.SetMuted(muted)
}

func (d *Device) Next() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.app == nil {
		return errDisconnected
	}
	return d.app.Next()
}

func (d *Device) Prev() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.app == nil {
		return errDisconnected
	}
	return d.app.Previous()
}

func (d *Device) Cast(url string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.app == nil {
		return errDisconnected
	}
	if err := d.app.Load(url, "", false, false); err != nil {
		return err
	}
	d.lastCastURL = url
	return nil
}

var errDisconnected = fmt.Errorf("device disconnected")
