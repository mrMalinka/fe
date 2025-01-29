package main

import (
	"os"

	"dario.cat/mergo"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Keybinds        map[string]string `yaml:"keybinds"`
	BuiltinKeybinds map[string]string `yaml:"builtin_keybinds"`

	AbsoluteKeybinds        map[string]string `yaml:"absolute_keybinds"`
	AbsoluteBuiltinKeybinds map[string]string `yaml:"absolute_builtin_keybinds"`

	Options struct {
		ShowHidden bool `yaml:"show_hidden"`
	} `yaml:"options"`
}

func getConfigPath() string {
	return os.ExpandEnv("$HOME/.config/fe/config.yaml")
}

func getDefaultConfig() Config {
	return Config{
		BuiltinKeybinds: map[string]string{
			"up":    "select_up",
			"down":  "select_down",
			"left":  "dir_backwards",
			"right": "dir_forwards",
		},

		AbsoluteBuiltinKeybinds: map[string]string{
			"q":   "quit",
			"esc": "clear_key_sequence",
		},

		Options: struct {
			ShowHidden bool `yaml:"show_hidden"`
		}{
			// TODO: actually implement showhidden = false
			ShowHidden: true,
		},
	}
}

func loadConfig() (*Config, error) {
	configPath := getConfigPath()
	defaultConfig := getDefaultConfig()
	userCfg := Config{}

	if _, err := os.Stat(configPath); err == nil {
		if data, err := os.ReadFile(configPath); err == nil {
			if err := yaml.Unmarshal(data, &userCfg); err != nil {
				return nil, err
			}
		}
	} else if !os.IsNotExist(err) {
		// do nothing if the file simply doesnt exist
		return nil, err
	}

	if err := mergo.Merge(&defaultConfig, userCfg); err != nil {
		return nil, err
	}

	return &defaultConfig, nil
}
