package main

import (
	"os"

	"dario.cat/mergo"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Keybinds         map[string]string `yaml:"keybinds"`
	AbsoluteKeybinds map[string]string `yaml:"absolute_keybinds"`

	Options struct {
		// whether to show files starting with `.`
		// TODO: add app command to do this with a keybind
		ShowHidden bool `yaml:"show_hidden"`

		// how long the key press sequence should persist before resetting in ms
		KeybindDuration int `yaml:"keybind_duration"`
	} `yaml:"options"`
}

func getConfigPath() string {
	return os.ExpandEnv("$HOME/.config/fe/config.yaml")
}

func getDefaultConfig() Config {
	return Config{
		Keybinds: map[string]string{
			"up":    "#select_up",
			"down":  "#select_down",
			"left":  "#dir_backwards",
			"right": "#dir_forwards",

			"ctrl+s": "#quit_cd",
		},

		AbsoluteKeybinds: map[string]string{
			"q":   "#quit",
			"esc": "#clear_key_sequence",
		},

		Options: struct {
			ShowHidden      bool `yaml:"show_hidden"`
			KeybindDuration int  `yaml:"keybind_duration"`
		}{
			// TODO: actually implement showhidden = false
			ShowHidden:      true,
			KeybindDuration: 600,
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
