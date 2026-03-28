package chromecast

import (
	"log"
	"time"

	"github.com/i574789/ottermediator/config"
)

// evaluateKeepalive is called from poll() while d.mu is held.
func evaluateKeepalive(d *Device) {
	dc := d.cfg.GetDevice(d.status.ID)
	if dc.KeepaliveMode == config.KeepaliveOff || dc.KeepaliveURL == "" {
		return
	}

	switch dc.KeepaliveMode {
	case config.KeepaliveKeepalive:
		// Relaunch only if transitioned to idle and the last cast was ours.
		if d.status.State == StateIdle &&
			d.prevState != StateDisconnected &&
			d.lastCastURL == dc.KeepaliveURL {
			scheduleRelaunch(d, dc.KeepaliveURL)
		}

	case config.KeepaliveForce:
		// Relaunch unless we are actively playing our URL.
		isOurs := d.status.State == StatePlaying && d.lastCastURL == dc.KeepaliveURL
		if !isOurs && d.status.State != StateDisconnected {
			scheduleRelaunch(d, dc.KeepaliveURL)
		}
	}
}

// scheduleRelaunch debounces a relaunch; d.mu must be held by the caller.
func scheduleRelaunch(d *Device, url string) {
	if d.relaunchTimer != nil {
		d.relaunchTimer.Stop()
	}
	d.relaunchTimer = time.AfterFunc(3*time.Second, func() {
		log.Printf("[%s] keep-alive: relaunching %s", d.status.Name, url)
		if err := d.Cast(url); err != nil {
			log.Printf("[%s] keep-alive relaunch failed: %v", d.status.Name, err)
		}
	})
}
