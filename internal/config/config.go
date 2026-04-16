// Package config provides centralized configuration loading for all ONVIF tutorial examples.
//
// It reads camera credentials and connection details from a .env.local file
// using the godotenv library. This keeps sensitive credentials (IP addresses,
// usernames, passwords) out of source code and version control.
//
// Usage:
//
//	cfg, err := config.Load()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(cfg.Host, cfg.Port, cfg.Username)
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/joho/godotenv"
)

// Config holds the primary ONVIF device connection parameters.
type Config struct {
	Host     string // IP address or hostname of the ONVIF device
	Port     string // ONVIF service port (usually 80 or 8080)
	Username string // Authentication username
	Password string // Authentication password
}

// CameraConfig holds connection parameters for a single camera
// in multi-camera scenarios.
type CameraConfig struct {
	Host     string
	Port     string
	Username string
	Password string
}

// Xaddr returns the "host:port" address string that the use-go/onvif library
// expects when creating a new Device.
func (c *Config) Xaddr() string {
	return c.Host + ":" + c.Port
}

// Xaddr returns the "host:port" address for a camera config.
func (c *CameraConfig) Xaddr() string {
	return c.Host + ":" + c.Port
}

// Load reads the .env.local file from the project root and returns
// the primary camera configuration. It searches for .env.local by
// walking up from the current file's directory to find the project root.
func Load() (*Config, error) {
	if err := loadEnvFile(); err != nil {
		return nil, err
	}

	cfg := &Config{
		Host:     getEnvOrDefault("ONVIF_HOST", "192.168.1.100"),
		Port:     getEnvOrDefault("ONVIF_PORT", "80"),
		Username: getEnvOrDefault("ONVIF_USERNAME", "admin"),
		Password: getEnvOrDefault("ONVIF_PASSWORD", "password"),
	}

	return cfg, nil
}

// LoadCameras reads multi-camera configuration from .env.local.
// It returns up to 3 camera configs (CAMERA_1_*, CAMERA_2_*, CAMERA_3_*).
// Cameras with empty host values are excluded from the result.
func LoadCameras() ([]*CameraConfig, error) {
	if err := loadEnvFile(); err != nil {
		return nil, err
	}

	var cameras []*CameraConfig
	for i := 1; i <= 3; i++ {
		prefix := fmt.Sprintf("CAMERA_%d_", i)
		host := os.Getenv(prefix + "HOST")
		if host == "" {
			continue
		}
		cameras = append(cameras, &CameraConfig{
			Host:     host,
			Port:     getEnvOrDefault(prefix+"PORT", "80"),
			Username: getEnvOrDefault(prefix+"USERNAME", "admin"),
			Password: getEnvOrDefault(prefix+"PASSWORD", "password"),
		})
	}

	if len(cameras) == 0 {
		return nil, fmt.Errorf("no cameras configured in .env.local (set CAMERA_1_HOST, etc.)")
	}

	return cameras, nil
}

// loadEnvFile finds and loads the .env.local file from the project root.
func loadEnvFile() error {
	// Try to find .env.local relative to the project root.
	// Walk up from the directory of the currently running source file.
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("cannot determine caller location")
	}

	// Start from this file's directory (internal/config/) and walk up
	dir := filepath.Dir(filename)
	for {
		envPath := filepath.Join(dir, ".env.local")
		if _, err := os.Stat(envPath); err == nil {
			return godotenv.Load(envPath)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Also try the current working directory
	if _, err := os.Stat(".env.local"); err == nil {
		return godotenv.Load(".env.local")
	}

	return fmt.Errorf("could not find .env.local — copy .env.example to .env.local and fill in your camera details")
}

func getEnvOrDefault(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
