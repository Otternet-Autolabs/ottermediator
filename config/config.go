package config

import (
	"encoding/json"
	"os"
	"sync"
)

type KeepaliveMode string

const (
	KeepaliveOff       KeepaliveMode = "off"
	KeepaliveKeepalive KeepaliveMode = "keepalive"
	KeepaliveForce     KeepaliveMode = "force"
)

type DeviceConfig struct {
	KeepaliveMode KeepaliveMode `json:"keepalive_mode"`
	KeepaliveURL  string        `json:"keepalive_url"`
}

type KnownDevice struct {
	Name string `json:"name"`
	Addr string `json:"addr"`
	Port int    `json:"port"`
}

type Config struct {
	mu           sync.RWMutex            `json:"-"`
	path         string                  `json:"-"`
	Devices      map[string]DeviceConfig `json:"devices"`
	KnownDevices map[string]KnownDevice  `json:"known_devices"`
}

func Load(path string) (*Config, error) {
	c := &Config{
		path:         path,
		Devices:      make(map[string]DeviceConfig),
		KnownDevices: make(map[string]KnownDevice),
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, c); err != nil {
		return nil, err
	}
	if c.Devices == nil {
		c.Devices = make(map[string]DeviceConfig)
	}
	if c.KnownDevices == nil {
		c.KnownDevices = make(map[string]KnownDevice)
	}
	return c, nil
}

func (c *Config) Save() error {
	c.mu.RLock()
	data, err := json.MarshalIndent(c, "", "  ")
	c.mu.RUnlock()
	if err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

func (c *Config) GetDevice(id string) DeviceConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if dc, ok := c.Devices[id]; ok {
		return dc
	}
	return DeviceConfig{KeepaliveMode: KeepaliveOff}
}

func (c *Config) SetDevice(id string, dc DeviceConfig) error {
	c.mu.Lock()
	c.Devices[id] = dc
	c.mu.Unlock()
	return c.Save()
}

func (c *Config) GetKnownDevices() map[string]KnownDevice {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]KnownDevice, len(c.KnownDevices))
	for k, v := range c.KnownDevices {
		out[k] = v
	}
	return out
}

func (c *Config) SetKnownDevice(id string, kd KnownDevice) error {
	c.mu.Lock()
	c.KnownDevices[id] = kd
	c.mu.Unlock()
	return c.Save()
}
