package chromecast

import (
	"context"
	"log"
	"net"
	"sync"
	"time"

	"github.com/i574789/ottermediator/config"
	"github.com/miekg/dns"
	castdns "github.com/vishen/go-chromecast/dns"
)

// DiscoveryManager discovers and manages all Chromecast devices.
type DiscoveryManager struct {
	mu      sync.RWMutex
	devices map[string]*Device
	cfg     *config.Config
	hub     Broadcaster
	iface   *net.Interface // nil = auto-detect
}

func NewDiscoveryManager(cfg *config.Config, hub Broadcaster, ifaceName string) *DiscoveryManager {
	var iface *net.Interface
	if ifaceName != "" {
		if i, err := net.InterfaceByName(ifaceName); err == nil {
			iface = i
			log.Printf("mDNS pinned to interface %s", ifaceName)
		} else {
			log.Printf("warning: interface %q not found, using auto-detect: %v", ifaceName, err)
		}
	}
	return &DiscoveryManager{
		devices: make(map[string]*Device),
		cfg:     cfg,
		hub:     hub,
		iface:   iface,
	}
}

// Start runs a persistent mDNS browse that catches announcements as they arrive.
func (dm *DiscoveryManager) Start(ctx context.Context) {
	// Immediately attempt to connect to devices seen in previous runs.
	dm.seedFromCache()
	for {
		log.Printf("mDNS browse starting...")
		entryCh, err := castdns.DiscoverCastDNSEntries(ctx, dm.iface)
		if err != nil {
			log.Printf("mDNS browse error: %v", err)
		} else {
			// Give the browse socket time to bind and join the multicast group,
			// then probe repeatedly so responses land on the zeroconf listener.
			go func() {
				for _, delay := range []time.Duration{300 * time.Millisecond, 1 * time.Second, 3 * time.Second} {
					time.Sleep(delay)
					probeMDNS(dm.iface)
				}
			}()
			for entry := range entryCh {
				dm.handleEntry(entry)
			}
		}
		// entryCh closed means ctx was cancelled or error — exit or retry
		select {
		case <-ctx.Done():
			return
		default:
			log.Printf("mDNS browse ended unexpectedly, restarting in 5s...")
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}
}

func (dm *DiscoveryManager) handleEntry(entry castdns.CastEntry) {
	id := entry.GetUUID()
	if id == "" || entry.GetName() == "" {
		return // incomplete record, wait for full announcement
	}

	// Persist updated address so next startup uses the current IP.
	dm.cfg.SetKnownDevice(id, config.KnownDevice{
		Name: entry.GetName(),
		Addr: entry.GetAddr(),
		Port: entry.GetPort(),
	})

	dm.mu.RLock()
	existing, exists := dm.devices[id]
	dm.mu.RUnlock()

	if exists {
		// Device already registered (possibly from cache) — just refresh its address.
		existing.UpdateEntry(entry)
		return
	}

	log.Printf("Discovered new device: %s (%s:%d)", entry.GetName(), entry.GetAddr(), entry.GetPort())
	dev := newDevice(entry, dm.cfg, dm.hub)

	dm.mu.Lock()
	dm.devices[id] = dev
	dm.mu.Unlock()

	dev.Start()
}

// seedFromCache registers devices from the persisted cache so they can start
// connecting before mDNS discovery completes.
func (dm *DiscoveryManager) seedFromCache() {
	known := dm.cfg.GetKnownDevices()
	for id, kd := range known {
		dm.mu.RLock()
		_, exists := dm.devices[id]
		dm.mu.RUnlock()
		if exists {
			continue
		}
		log.Printf("Seeding device from cache: %s (%s:%d)", kd.Name, kd.Addr, kd.Port)
		entry := castdns.CastEntry{
			AddrV4:     net.ParseIP(kd.Addr),
			Port:       kd.Port,
			UUID:       id,
			DeviceName: kd.Name,
		}
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

// probeMDNS sends a multicast DNS PTR query for _googlecast._tcp.local to
// solicit immediate responses from all Chromecasts on the network, rather
// than waiting for the zeroconf library's 4-second probe backoff.
func probeMDNS(iface *net.Interface) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return
	}
	defer conn.Close()

	m := new(dns.Msg)
	m.SetQuestion("_googlecast._tcp.local.", dns.TypePTR)
	m.RecursionDesired = false
	// No QU bit — responses go to multicast 224.0.0.251:5353 where zeroconf listens.

	buf, err := m.Pack()
	if err != nil {
		return
	}

	mdnsAddr := &net.UDPAddr{IP: net.ParseIP("224.0.0.251"), Port: 5353}
	// Send the probe 3 times with short gaps so we catch devices that miss the first one.
	for i := 0; i < 3; i++ {
		conn.WriteTo(buf, mdnsAddr)
		if i < 2 {
			time.Sleep(200 * time.Millisecond)
		}
	}
}
