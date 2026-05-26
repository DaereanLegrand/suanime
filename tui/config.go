package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	DownloadDir    string `json:"download_dir"`
	Aria2RPCPort   int    `json:"aria2_rpc_port"`
	Aria2RPCSecret string `json:"aria2_rpc_secret"`
}

func DefaultConfig() Config {
	home, _ := os.UserHomeDir()
	dl := filepath.Join(home, "Downloads", "suanime")
	return Config{
		DownloadDir:    dl,
		Aria2RPCPort:   6800,
		Aria2RPCSecret: "",
	}
}

func configDir() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		home, err2 := os.UserHomeDir()
		if err2 != nil {
			return "", fmt.Errorf("cannot find config directory")
		}
		d = filepath.Join(home, ".config")
	}
	dir := filepath.Join(d, "suanime")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("cannot create config dir: %w", err)
	}
	return dir, nil
}

func LoadConfig() (Config, error) {
	dir, err := configDir()
	if err != nil {
		return DefaultConfig(), err
	}
	path := filepath.Join(dir, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		cfg := DefaultConfig()
		saveConfig(cfg)
		return cfg, nil
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig(), err
	}
	if cfg.DownloadDir == "" {
		cfg.DownloadDir = DefaultConfig().DownloadDir
	}
	if cfg.Aria2RPCPort == 0 {
		cfg.Aria2RPCPort = 6800
	}
	return cfg, nil
}

func saveConfig(cfg Config) error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(filepath.Join(dir, "config.json"), data, 0644)
}

func ConfigFilePath() string {
	dir, err := configDir()
	if err != nil {
		return "~/.config/suanime/config.json"
	}
	return filepath.Join(dir, "config.json")
}
