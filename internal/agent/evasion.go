package agent

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type SandboxIndicators struct {
	LowCPU       bool
	LowMemory    bool
	RecentBoot   bool
	DebuggerPID  bool
	KnownVM      bool
	Suspicious   int
}

func CheckSandbox() *SandboxIndicators {
	ind := &SandboxIndicators{}

	if runtime.NumCPU() <= 2 {
		ind.LowCPU = true
		ind.Suspicious++
	}

	if runtime.GOOS == "linux" {
		if data, err := os.ReadFile("/proc/meminfo"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "MemTotal:") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						if kb, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
							if kb < 2*1024*1024 {
								ind.LowMemory = true
								ind.Suspicious++
							}
						}
					}
				}
			}
		}

		if data, err := os.ReadFile("/proc/uptime"); err == nil {
			fields := strings.Fields(string(data))
			if len(fields) >= 1 {
				if uptime, err := strconv.ParseFloat(fields[0], 64); err == nil {
					if uptime < float64(15*time.Minute/time.Second) {
						ind.RecentBoot = true
						ind.Suspicious++
					}
				}
			}
		}

		if data, err := os.ReadFile("/proc/self/status"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "TracerPid:") {
					fields := strings.Fields(line)
					if len(fields) >= 2 && fields[1] != "0" {
						ind.DebuggerPID = true
						ind.Suspicious++
					}
				}
			}
		}

		vmIndicators := []string{
			"/sys/class/dmi/id/product_name",
			"/sys/class/dmi/id/sys_vendor",
		}
		vmStrings := []string{"virtualbox", "vmware", "qemu", "kvm", "xen", "bochs"}
		for _, path := range vmIndicators {
			if data, err := os.ReadFile(path); err == nil {
				lower := strings.ToLower(string(data))
				for _, vm := range vmStrings {
					if strings.Contains(lower, vm) {
						ind.KnownVM = true
						ind.Suspicious++
						break
					}
				}
			}
		}
	}

	return ind
}

func (s *SandboxIndicators) IsSuspicious() bool {
	return s.Suspicious >= 3
}
