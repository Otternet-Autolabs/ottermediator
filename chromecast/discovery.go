package chromecast

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/i574789/ottermediator/config"
	castdns "github.com/vishen/go-chromecast/dns"
)

// DiscoveryManager discovers and manages all Chromecast devices.
type DiscoveryManager struct {
	mu      sync.RWMutex
	devices map[string]*Device
	cfg     *config.Config
	hub     Broadcaster
}

func NewDiscoveryManager(cfg *config.Config, hub Broadcaster) *DiscoveryManager {
	return &DiscoveryManager{
		devices: make(map[string]*Device),
		cfg:     cfg,
		hub:     hub,
	}
}

// Start runs mDNS discovery on a 30-second interval.
func (dm *DiscoveryManager) Start(ctx context.Context) {
	dm.scan(ctx)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			dm.scan(ctx)
		}
	}
}

func (dm *DiscoveryManager) scan(ctx context.Context) {
	scanCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	entryCh, err := castdns.DiscoverCastDNSEntries(scanCtx, nil)
	if err != nil {
		log.Printf("mDNS scan error: %v", err)
		return
	}

	for entry := range entryCh {
		id := entry.GetUUID()
		dm.mu.RLock()
		_, exists := dm.devices[id]
		dm.mu.RUnlock()

		if exists {
			continue
		}

		log.Printf("Discovered new device: %s (%s:%d)", entry.GetName(), entry.GetAddr(), entry.GetPort())
		dev := newDevice(entry, dm.cfg, dm.hub)

		dm.mu.Lock()
		dm.devices[id] = dev
		dm.mu.Unlock()

		dev.Start()
	}
}

// AllStatuses returns a snapshot of all known devices.
func (dm *DiscoveryManager) AllStatuses() []DeviceStatus {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	statuses := make([]DeviceStatus, 0, len(dm.devices))
	for _, dev := range dm.devices {
		statuses = append(statuses, dev.GetStatus())
	}
	return statuses
}

// GetDevice returns the device with the given ID.
func (dm *DiscoveryManager) GetDevice(id string) (*Device, bool) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	dev, ok := dm.devices[id]
	return dev, ok
}
