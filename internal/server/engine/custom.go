package engine

import (
	"encoding/json"
	"log"
)

type customConfig struct {
	Command string `json:"command"`
	Type    string `json:"type"`
}

func CustomTasks(config string) []TaskTemplate {
	var cfg customConfig
	if err := json.Unmarshal([]byte(config), &cfg); err != nil {
		log.Printf("[-] Custom: invalid config: %v", err)
		return nil
	}

	if cfg.Command == "" {
		return nil
	}
	if cfg.Type == "" {
		cfg.Type = "shell"
	}

	return []TaskTemplate{
		{
			Name:    "Custom Command",
			CmdType: cfg.Type,
			Payload: cfg.Command,
		},
	}
}
