package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	RPCURL  string `json:"rpc_url" yaml:"rpc_url"`
	RPCUser string `json:"rpc_user" yaml:"rpc_user"`
	RPCPass string `json:"rpc_pass" yaml:"rpc_pass"`
	Wallet  string `json:"wallet" yaml:"wallet"`
	Network string `json:"network" yaml:"network"`
}

// DefaultPath returns the default config file path under the user's home directory.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".dustcleaner.yaml"
	}
	return filepath.Join(home, ".dustcleaner.yaml")
}

// Load reads configuration from the supplied path.
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{
				RPCURL:  "http://127.0.0.1:8332",
				Network: "mainnet",
			}, nil
		}
		return nil, fmt.Errorf("read config file %q: %w", path, err)
	}

	cfg := &Config{}

	if err := yaml.Unmarshal(data, cfg); err == nil {
		return cfg, nil
	}

	if err := json.Unmarshal(data, cfg); err == nil {
		return cfg, nil
	}

	return nil, fmt.Errorf("failed to parse config file %q as YAML or JSON", path)
}
