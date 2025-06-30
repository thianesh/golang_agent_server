package hystersisloadmanagement

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"testing"
	"thianesh/web_server/models"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// Struct to store system resource usage
type SystemConsumption struct {
	CPUPercent float64
	RAMTotal   float64
	RAMUsed    float64
	RAMPercent float64
}

// Monitor system usage every interval seconds and update the given pointer with write lock
func StartSystemMonitor(consumption *SystemConsumption, mu *sync.RWMutex) {
	interval := getIntervalFromEnv()

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		updateSystemConsumption(consumption, mu)
	}
}

func StartSystemMonitorAndSendAnalytics(consumption *SystemConsumption, mu *sync.RWMutex, companySFUs *map[string]*models.CompanySFU, companySFUsMutex *sync.RWMutex) {
	interval := getIntervalFromEnv()

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		updateSystemConsumption(consumption, mu)
		insertAnalytics(companySFUs, companySFUsMutex, consumption, mu)
	}
}

// Helper to read interval from env (default: 5s)
func getIntervalFromEnv() int {
	intervalStr := os.Getenv("SYSTEM_MONITOR_INTERVAL")
	interval, err := strconv.Atoi(intervalStr)
	if err != nil || interval <= 0 {
		return 15 // default
	}
	return interval
}

// Collect CPU and memory stats and update the struct
func updateSystemConsumption(consumption *SystemConsumption, mu *sync.RWMutex) {
	cpuPercent, err := cpu.Percent(time.Second, false)
	if err != nil || len(cpuPercent) == 0 {
		fmt.Println("Error getting CPU usage:", err)
		return
	}

	vmStat, err := mem.VirtualMemory()
	if err != nil {
		fmt.Println("Error getting memory usage:", err)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	consumption.CPUPercent = cpuPercent[0]
	consumption.RAMTotal = float64(vmStat.Total) / 1e9
	consumption.RAMUsed = float64(vmStat.Used) / 1e9
	consumption.RAMPercent = vmStat.UsedPercent
}

func TestStartSystemMonitor(t *testing.T) {
	var usage SystemConsumption
	var mu sync.RWMutex

	go StartSystemMonitor(&usage, &mu)

	time.Sleep(15 * time.Second)

	mu.RLock()
	defer mu.RUnlock()
	fmt.Printf("CPU: %.2f%% | RAM: %.2fGB / %.2fGB (%.2f%%)\n",
		usage.CPUPercent, usage.RAMUsed, usage.RAMTotal, usage.RAMPercent)
}

type metadataStruct struct {
	CPU       int      `json:"cpu"`
	RAM       int      `json:"ram"`
	AudioPipe []string `json:"audio_pipe,omitempty"`
	VideoPipe []string `json:"video_pipe,omitempty"`
}

type analyticsPayload struct {
	CompanyID string         `json:"company_id,omitempty"`
	UserID    string         `json:"user_id,omitempty"`
	Email     string         `json:"email,omitempty"`
	Metadata  metadataStruct `json:"metadata,omitempty"`
	Type      string         `json:"type"`
}

func insertAnalytics(companySFUs *map[string]*models.CompanySFU, companySFUsMutex *sync.RWMutex, consumption *SystemConsumption, mu *sync.RWMutex) {
	url := "https://db.vldo.in/functions/v1/analytics"

	var jsonData []analyticsPayload

	companySFUsMutex.RLock()
	for _, companySFU := range *companySFUs {
		for _, user := range companySFU.Users {
			jsonData = append(jsonData, analyticsPayload{
				CompanyID: companySFU.CompanyID,
				UserID:    string(user.UserId),
				Email:     user.Email,
				Type:      "user_data",
			})
		}
	}
	companySFUsMutex.RUnlock()

	mu.RLock()
	jsonData = append(jsonData, analyticsPayload{
		Type: "system_data",
		Metadata: metadataStruct{
			CPU: int(consumption.CPUPercent),
			RAM: int(consumption.RAMPercent),
		},
	})
	mu.RUnlock()

	payloadBytes, err := json.Marshal(jsonData)
	if err != nil {
		fmt.Println("Unable to marshal the payload in analytics:", err)
		return
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(payloadBytes))
	if err != nil {
		fmt.Println("Error sending request:", err)
		return
	}
	defer resp.Body.Close()

	fmt.Println("Status code:", resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response:", err)
		return
	}

	fmt.Println("Response body:", string(body))

	if resp.StatusCode != http.StatusOK {
		fmt.Println("Something went wrong...")
	}
}
