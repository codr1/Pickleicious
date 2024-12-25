// ./internal/config/config.go
package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App struct {
		Name string `yaml:"name"`
		Port string `yaml:"port"`
		Env  string `yaml:"env"`
	} `yaml:"app"`

	Database struct {
		Path string `yaml:"path"`
	} `yaml:"database"`

	Logging struct {
		Level string `yaml:"level"`
	} `yaml:"logging"`

	Features struct {
		Profiling  bool `yaml:"profiling"`
		DebugTools bool `yaml:"debug_tools"`
	} `yaml:"features"`
}

// Load loads the configuration from the specified path
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// LoadDefault attempts to load configuration from the default location
func LoadDefault() (*Config, error) {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		// Look for config in standard locations
		candidates := []string{
			"./config/app.yaml",
			"/etc/pickleicious/app.yaml",
		}

		for _, path := range candidates {
			if _, err := os.Stat(path); err == nil {
				configPath = path
				break
			}
		}

		if configPath == "" {
			return nil, fmt.Errorf("no configuration file found")
		}
	}

	return Load(configPath)
}
