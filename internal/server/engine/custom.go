package engine

import "encoding/json"

type customConfig struct {
	Command string `json:"command"`
	Type    string `json:"type"`
}

func CustomTasks(config string) []TaskTemplate {
	var cfg customConfig
	json.Unmarshal([]byte(config), &cfg)

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
