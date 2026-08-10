package hardware

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

type GPUInfo struct {
	HasGPU       bool   `json:"has_gpu"`
	Vendor       string `json:"vendor"`        // "nvidia", "amd", "intel", "apple", or "cpu-only"
	Model        string `json:"model"`         // e.g., "Radeon RX 7700 XT", "GeForce RTX 4090", "Apple M3 Pro"
	TotalVRAMGB  int    `json:"total_vram_gb"` // Total dedicated or unified memory in GB
	ToolkitReady bool   `json:"toolkit_ready"` // The Guardrail Lock
}

type SystemProfile struct {
	TotalRAMGB int     `json:"total_ram_gb"`
	CPUCores   int     `json:"cpu_cores"`
	CPUName    string  `json:"cpu_name"`
	GPU        GPUInfo `json:"gpu"`
}

// RunProfiler runs a comprehensive multi-vendor hardware check, filtering out non-accelerated iGPUs.
func RunProfiler() (*SystemProfile, error) {
	profile := &SystemProfile{
		GPU: GPUInfo{
			HasGPU: false,
			Vendor: "cpu-only",
			Model:  "CPU Only",
		},
	}

	// 1. Memory Check
	if vMem, err := mem.VirtualMemory(); err == nil {
		profile.TotalRAMGB = int(vMem.Total / (1024 * 1024 * 1024))
	}

	// 2. CPU Check (Fixed to report total logical threads correctly)
	profile.CPUCores = runtime.NumCPU()
	if cpuInfo, err := cpu.Info(); err == nil && len(cpuInfo) > 0 {
		profile.CPUName = strings.TrimSpace(cpuInfo[0].ModelName)
	} else {
		profile.CPUName = "Generic Processor"
	}

	// 3. Multi-Vendor GPU Pipeline (Only catches real discrete/accelerated GPUs)
	detectUsableGPU(&profile.GPU)

	return profile, nil
}

func detectUsableGPU(gpu *GPUInfo) {
	// Priority 1: Apple Silicon (macOS unified memory)
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		gpu.HasGPU = true
		gpu.Vendor = "apple"
		gpu.Model = "Apple Silicon (Unified Memory)"
		return
	}

	// Priority 2: Dedicated NVIDIA GPUs (via nvidia-smi)
	if checkNvidia(gpu) {
		return
	}

	// Priority 3: Dedicated AMD GPUs with actual VRAM
	if checkDiscreteAMD(gpu) {
		return
	}

	// Priority 4: Dedicated Intel Arc GPUs (ignoring integrated iGPUs)
	if checkDiscreteIntel(gpu) {
		return
	}

	// If no discrete accelerator is found, it explicitly remains "CPU Only"
}

func detectGPU(gpu *GPUInfo, systemRAMGB int) {
	// Priority 1: Apple Silicon (macOS)
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		gpu.HasGPU = true
		gpu.Vendor = "apple"
		gpu.Model = "Apple Silicon (Unified Memory)"
		gpu.TotalVRAMGB = systemRAMGB // Apple Metal utilizes unified memory
		return
	}

	// Priority 2: NVIDIA GPUs (via nvidia-smi)
	if checkNvidia(gpu) {
		return
	}

	// Priority 3: AMD GPUs (via rocm-smi or SysFS)
	if checkAMD(gpu) {
		return
	}

	// Priority 4: Intel GPUs (via SYCL or SysFS)
	if checkIntel(gpu) {
		return
	}

	// Priority 5: Generic Linux SysFS DRM Inspection (Catches unlisted GPUs)
	if checkLinuxSysFS(gpu) {
		return
	}
}

// --- GPU VENDOR PROBES ---

func checkNvidia(gpu *GPUInfo) bool {
	cmd := exec.Command("nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader,nounits")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	parts := strings.Split(strings.TrimSpace(string(output)), ",")
	if len(parts) >= 2 {
		gpu.HasGPU = true
		gpu.Vendor = "nvidia"
		gpu.Model = strings.TrimSpace(parts[0])

		if vramMB, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
			gpu.TotalVRAMGB = vramMB / 1024
		}

		// 🚨 THE HARD GUARDRAIL
		// Check if the Docker-NVIDIA toolkit is installed on the host OS
		if _, err := exec.LookPath("nvidia-container-cli"); err == nil {
			gpu.ToolkitReady = true
		} else {
			gpu.ToolkitReady = false
		}

		return true
	}
	return false
}

func checkAMD(gpu *GPUInfo) bool {
	// Try rocm-smi CLI first
	cmd := exec.Command("rocm-smi", "--showproductname", "--showmeminfo", "vram", "--csv")
	output, err := cmd.Output()
	if err == nil && strings.Contains(string(output), "card") {
		gpu.HasGPU = true
		gpu.Vendor = "amd"
		gpu.Model = "AMD Radeon GPU (ROCm Enabled)"
		gpu.TotalVRAMGB = parseAMDVram(string(output))
		return true
	}

	// Fallback to reading Linux SysFS directly for AMD cards (Vendor ID: 0x1002)
	cards, _ := filepath.Glob("/sys/class/drm/card*/device/vendor")
	for _, cardVendorPath := range cards {
		vendorBytes, err := os.ReadFile(cardVendorPath)
		if err == nil && strings.TrimSpace(string(vendorBytes)) == "0x1002" {
			gpu.HasGPU = true
			gpu.Vendor = "amd"
			gpu.Model = "AMD Radeon GPU"

			// Try reading VRAM directly from sysfs
			vramPath := filepath.Join(filepath.Dir(cardVendorPath), "mem_info_vram_total")
			if vramBytes, err := os.ReadFile(vramPath); err == nil {
				if vram, err := strconv.ParseUint(strings.TrimSpace(string(vramBytes)), 10, 64); err == nil {
					gpu.TotalVRAMGB = int(vram / (1024 * 1024 * 1024))
				}
			}
			return true
		}
	}
	return false
}

func checkIntel(gpu *GPUInfo) bool {
	// Intel Arc / Iris Xe check via SysFS (Vendor ID: 0x8086)
	cards, _ := filepath.Glob("/sys/class/drm/card*/device/vendor")
	for _, cardVendorPath := range cards {
		vendorBytes, err := os.ReadFile(cardVendorPath)
		if err == nil && strings.TrimSpace(string(vendorBytes)) == "0x8086" {
			// Check if it has dedicated VRAM (Arc GPU vs Integrated)
			vramPath := filepath.Join(filepath.Dir(cardVendorPath), "mem_info_vram_total")
			if vramBytes, err := os.ReadFile(vramPath); err == nil {
				if vram, err := strconv.ParseUint(strings.TrimSpace(string(vramBytes)), 10, 64); err == nil {
					vramGB := int(vram / (1024 * 1024 * 1024))
					if vramGB > 2 { // Dedicated Intel Arc card
						gpu.HasGPU = true
						gpu.Vendor = "intel"
						gpu.Model = "Intel Arc / OneAPI GPU"
						gpu.TotalVRAMGB = vramGB
						return true
					}
				}
			}
		}
	}
	return false
}

func checkLinuxSysFS(gpu *GPUInfo) bool {
	// Fallback lspci for any VGA/3D controller
	cmd := exec.Command("bash", "-c", "lspci | grep -iE 'vga|3d|display'")
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		line := string(output)
		gpu.HasGPU = true
		if strings.Contains(line, "AMD") || strings.Contains(line, "Radeon") {
			gpu.Vendor = "amd"
			gpu.Model = "AMD Radeon (PCIe)"
		} else if strings.Contains(line, "NVIDIA") {
			gpu.Vendor = "nvidia"
			gpu.Model = "NVIDIA GPU (PCIe)"
		} else if strings.Contains(line, "Intel") {
			gpu.Vendor = "intel"
			gpu.Model = "Intel Display Controller"
		} else {
			gpu.Vendor = "generic"
			gpu.Model = "Generic GPU Controller"
		}
		return true
	}
	return false
}

func parseAMDVram(_ string) int {
	// Default fallback to 8GB if ROCm outputs complex CSV
	return 8
}

func checkDiscreteAMD(gpu *GPUInfo) bool {
	// Fallback to reading Linux SysFS directly for AMD cards, requiring actual VRAM > 2GB
	cards, _ := filepath.Glob("/sys/class/drm/card*/device/vendor")
	for _, cardVendorPath := range cards {
		vendorBytes, err := os.ReadFile(cardVendorPath)
		if err == nil && strings.TrimSpace(string(vendorBytes)) == "0x1002" {
			vramPath := filepath.Join(filepath.Dir(cardVendorPath), "mem_info_vram_total")
			if vramBytes, err := os.ReadFile(vramPath); err == nil {
				if vram, err := strconv.ParseUint(strings.TrimSpace(string(vramBytes)), 10, 64); err == nil {
					vramGB := int(vram / (1024 * 1024 * 1024))
					if vramGB > 2 { // Only accept if it's a discrete card with substantial VRAM
						gpu.HasGPU = true
						gpu.Vendor = "amd"
						gpu.Model = "AMD Radeon Discrete GPU"
						gpu.TotalVRAMGB = vramGB
						return true
					}
				}
			}
		}
	}
	return false
}

func checkDiscreteIntel(gpu *GPUInfo) bool {
	// Only flag Intel if it has dedicated VRAM (e.g., Intel Arc discrete cards, ignoring iGPUs)
	cards, _ := filepath.Glob("/sys/class/drm/card*/device/vendor")
	for _, cardVendorPath := range cards {
		vendorBytes, err := os.ReadFile(cardVendorPath)
		if err == nil && strings.TrimSpace(string(vendorBytes)) == "0x8086" {
			vramPath := filepath.Join(filepath.Dir(cardVendorPath), "mem_info_vram_total")
			if vramBytes, err := os.ReadFile(vramPath); err == nil {
				if vram, err := strconv.ParseUint(strings.TrimSpace(string(vramBytes)), 10, 64); err == nil {
					vramGB := int(vram / (1024 * 1024 * 1024))
					if vramGB > 2 { // Dedicated Intel Arc card
						gpu.HasGPU = true
						gpu.Vendor = "intel"
						gpu.Model = "Intel Arc GPU"
						gpu.TotalVRAMGB = vramGB
						return true
					}
				}
			}
		}
	}
	return false
}

// --- EVALUATION ENGINE ---

// func evaluateAIRecommendation(p *SystemProfile) string {
// 	if p.GPU.HasGPU {
// 		vram := p.GPU.TotalVRAMGB
// 		switch p.GPU.Vendor {
// 		case "nvidia":
// 			if vram >= 16 {
// 				return "ollama-gpu-high (CUDA)"
// 			}
// 			return "ollama-gpu-standard (CUDA)"
// 		case "amd":
// 			return fmt.Sprintf("ollama-gpu-rocm (%dGB VRAM)", vram)
// 		case "apple":
// 			return "ollama-metal-unified"
// 		case "intel":
// 			return "ollama-sycl-intel"
// 		}
// 	}

// 	// CPU Fallbacks
// 	if p.TotalRAMGB >= 16 {
// 		return "ollama-cpu-standard (16GB RAM)"
// 	} else if p.TotalRAMGB >= 8 {
// 		return "ollama-cpu-light (8GB RAM)"
// 	}

// 	return "unsupported"
// }
