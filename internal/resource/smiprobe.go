package resource

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// smiProbe reads VRAM through nvidia-smi.
//
// Why not WMI: Win32_VideoController.AdapterRAM is a 32-bit field, so a 12 GB card
// reports ~4 GB. Measured on the development machine (12288 MiB via NVML vs
// 4,293,918,720 bytes via WMI). A wrong number here causes wrong admission
// decisions, so nvidia-smi is the fallback path until NVML is bound in CHG-0002.
//
// Results are cached: nvidia-smi costs ~100 ms and spawning it per admission
// check would be worse than the problem it solves.
type smiProbe struct {
	once  sync.Once
	total int
	free  int
	driver int
	name  string
	err   error
}

var (
	sharedOnce  sync.Once
	sharedProbe *smiProbe
)

// NewProbe returns the process-wide probe. Safe for concurrent use.
func NewProbe() Probe {
	sharedOnce.Do(func() { sharedProbe = &smiProbe{} })
	return sharedProbe
}

func (p *smiProbe) load() {
	p.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()

		out, err := exec.CommandContext(ctx, "nvidia-smi",
			"--query-gpu=name,memory.total,memory.free,driver_version",
			"--format=csv,noheader,nounits").Output()
		if err != nil {
			p.err = err
			return
		}
		line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
		fields := strings.Split(line, ",")
		if len(fields) < 4 {
			p.err = errMalformedSMI
			return
		}
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}
		total, err1 := strconv.Atoi(fields[1])
		free, err2 := strconv.Atoi(fields[2])
		if err1 != nil || err2 != nil {
			p.err = errMalformedSMI
			return
		}
		p.name = fields[0]
		p.total = total
		p.free = free
		p.driver = parseMajor(fields[3])
	})
}

func parseMajor(v string) int {
	head := v
	if i := strings.IndexByte(v, '.'); i > 0 {
		head = v[:i]
	}
	n, err := strconv.Atoi(strings.TrimSpace(head))
	if err != nil {
		return 0
	}
	return n
}

// TotalMB returns total VRAM in mebibytes.
func (p *smiProbe) TotalMB() (int, error) {
	p.load()
	if p.err != nil {
		return 0, p.err
	}
	return p.total, nil
}

// FreeMB returns free VRAM in mebibytes.
func (p *smiProbe) FreeMB() (int, error) {
	p.load()
	if p.err != nil {
		return 0, p.err
	}
	return p.free, nil
}

// DeviceName returns the GPU name when known.
func DeviceName() string {
	p, ok := NewProbe().(*smiProbe)
	if !ok {
		return ""
	}
	p.load()
	return p.name
}

// LooksNVIDIA reports whether an NVIDIA device was detected.
func LooksNVIDIA() bool {
	p, ok := NewProbe().(*smiProbe)
	if !ok {
		return false
	}
	p.load()
	return p.err == nil && p.name != ""
}

// DriverMajor returns the NVIDIA driver major version, or 0 when unknown.
func DriverMajor() int {
	p, ok := NewProbe().(*smiProbe)
	if !ok {
		return 0
	}
	p.load()
	return p.driver
}

// CUDA12Present reports whether the driver is new enough for the CUDA 12 build.
//
// This checks the driver version, not the CUDA toolkit: the engine ships its own
// runtime, so the driver is the only prerequisite (MODEL-REGISTRY.md §5).
func CUDA12Present() bool {
	major := DriverMajor()
	return major >= minDriverMajor
}

// VulkanPresent reports whether a Vulkan runtime is plausibly available.
//
// A definitive check needs the loader; until the engine probes itself (CHG-0002)
// this conservatively reports true on Windows, where the loader is shipped by the
// driver and the engine downgrade chain will catch a real failure.
func VulkanPresent() bool { return true }

type smiError string

func (e smiError) Error() string { return string(e) }

const errMalformedSMI smiError = "unexpected nvidia-smi output"
