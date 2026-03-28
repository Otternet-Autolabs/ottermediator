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

type Config struct {
	mu      sync.RWMutex            `json:"-"`
	path    string                  `json:"-"`
	Devices map[string]DeviceConfig `json:"devices"`
}

func Load(path string) (*Config, error) {
	c := &Config{
		path:    path,
		Devices: make(map[string]DeviceConfig),
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
