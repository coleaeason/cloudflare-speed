package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/coleaeason/cloudflare-speed/internal/log"
	"github.com/coleaeason/cloudflare-speed/internal/math"
)

func main() {
	fmt.Println("Cloudflare Speed Test")
	if err := speedTest(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// --- HTTP client functionality ---
func get(hostname, path string) ([]byte, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	url := fmt.Sprintf("https://%s%s", hostname, path)
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func fetchServerLocationData() (map[string]string, error) {
	data, err := get("speed.cloudflare.com", "/locations")
	if err != nil {
		return nil, err
	}

	var locations []struct {
		IATA string `json:"iata"`
		City string `json:"city"`
	}
	if err := json.Unmarshal(data, &locations); err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for _, loc := range locations {
		result[loc.IATA] = loc.City
	}
	return result, nil
}

func fetchCfCdnCgiTrace() (map[string]string, error) {
	data, err := get("speed.cloudflare.com", "/cdn-cgi/trace")
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	result := make(map[string]string)
	for _, line := range lines {
		parts := strings.Split(line, "=")
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result, nil
}

type requestTiming struct {
	started      time.Time
	dnsLookup    time.Time
	tcpHandshake time.Time
	sslHandshake time.Time
	ttfb         time.Time
	ended        time.Time
	serverTiming float64
}

func request(method, hostname, path string, data []byte) (*requestTiming, error) {
	timing := &requestTiming{
		started: time.Now(),
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false,
			},
		},
	}

	req, err := http.NewRequest(method, fmt.Sprintf("https://%s%s", hostname, path), strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}

	if len(data) > 0 {
		req.Header.Set("Content-Length", strconv.Itoa(len(data)))
	}

	trace := &httptrace.ClientTrace{
		DNSDone: func(dnsInfo httptrace.DNSDoneInfo) {
			timing.dnsLookup = time.Now()
		},
		ConnectDone: func(network, addr string, err error) {
			timing.tcpHandshake = time.Now()
		},
		TLSHandshakeDone: func(state tls.ConnectionState, err error) {
			timing.sslHandshake = time.Now()
		},
		GotFirstResponseByte: func() {
			timing.ttfb = time.Now()
		},
	}

	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read the entire response to ensure timing.ended is accurate
	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		return nil, err
	}

	timing.ended = time.Now()

	// Parse server timing header if available
	if serverTiming := resp.Header.Get("Server-Timing"); serverTiming != "" {
		parts := strings.Split(serverTiming, ";")
		if len(parts) > 1 {
			durPart := strings.TrimSpace(parts[1])
			if strings.HasPrefix(durPart, "dur=") {
				if val, err := strconv.ParseFloat(durPart[4:], 64); err == nil {
					timing.serverTiming = val
				}
			}
		}
	}

	return timing, nil
}

func download(bytes int) (*requestTiming, error) {
	return request("GET", "speed.cloudflare.com", fmt.Sprintf("/__down?bytes=%d", bytes), nil)
}

func upload(bytes int) (*requestTiming, error) {
	data := strings.Repeat("0", bytes)
	return request("POST", "speed.cloudflare.com", "/__up", []byte(data))
}

func measureSpeed(bytes int, duration time.Duration) float64 {
	return float64(bytes*8) / (duration.Seconds() * 1e6)
}

func measureLatency() ([]float64, error) {
	var measurements []float64

	for i := 0; i < 20; i++ {
		timing, err := download(1000)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		// TTFB - Server processing time
		latency := timing.ttfb.Sub(timing.started).Seconds()*1000 - timing.serverTiming
		measurements = append(measurements, latency)
	}

	min := measurements[0]
	max := measurements[0]
	for _, v := range measurements {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}

	return []float64{min, max, math.Average(measurements), math.Median(measurements), math.Jitter(measurements)}, nil
}

func measureDownload(bytes, iterations int) ([]float64, error) {
	var measurements []float64

	for i := 0; i < iterations; i++ {
		timing, err := download(bytes)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		transferTime := timing.ended.Sub(timing.ttfb)
		measurements = append(measurements, measureSpeed(bytes, transferTime))
	}

	return measurements, nil
}

func measureUpload(bytes, iterations int) ([]float64, error) {
	var measurements []float64

	for i := 0; i < iterations; i++ {
		timing, err := upload(bytes)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		transferTime := time.Duration(timing.serverTiming * float64(time.Millisecond))
		measurements = append(measurements, measureSpeed(bytes, transferTime))
	}

	return measurements, nil
}

func speedTest() error {
	pingResults, err := measureLatency()
	if err != nil {
		return fmt.Errorf("failed to measure latency: %w", err)
	}

	serverLocationData, err := fetchServerLocationData()
	if err != nil {
		return fmt.Errorf("failed to fetch server location data: %w", err)
	}

	traceData, err := fetchCfCdnCgiTrace()
	if err != nil {
		return fmt.Errorf("failed to fetch CDN trace: %w", err)
	}

	city := serverLocationData[traceData["colo"]]
	log.PrintPair("Server location", fmt.Sprintf("%s (%s)", city, traceData["colo"]), log.Blue)
	log.PrintPair("Your IP", fmt.Sprintf("%s (%s)", traceData["ip"], traceData["loc"]), log.Blue)

	// Print latency information
	log.PrintFloat("Latency", pingResults[3], 2, "ms", log.Magenta)
	log.PrintFloat("Jitter", pingResults[4], 2, "ms", log.Magenta)

	// Download tests
	testDown1, err := measureDownload(101000, 10)
	if err != nil {
		return fmt.Errorf("failed to measure 100kB download: %w", err)
	}
	log.PrintFloat("100kB speed", math.Median(testDown1), 2, "Mbps", log.Yellow)

	testDown2, err := measureDownload(1001000, 8)
	if err != nil {
		return fmt.Errorf("failed to measure 1MB download: %w", err)
	}
	log.PrintFloat("1MB speed", math.Median(testDown2), 2, "Mbps", log.Yellow)

	testDown3, err := measureDownload(10001000, 6)
	if err != nil {
		return fmt.Errorf("failed to measure 10MB download: %w", err)
	}
	log.PrintFloat("10MB speed", math.Median(testDown3), 2, "Mbps", log.Yellow)

	testDown4, err := measureDownload(25001000, 4)
	if err != nil {
		return fmt.Errorf("failed to measure 25MB download: %w", err)
	}
	log.PrintFloat("25MB speed", math.Median(testDown4), 2, "Mbps", log.Yellow)

	testDown5, err := measureDownload(100001000, 1)
	if err != nil {
		return fmt.Errorf("failed to measure 100MB download: %w", err)
	}
	log.PrintFloat("100MB speed", math.Median(testDown5), 2, "Mbps", log.Yellow)

	downloadTests := append(append(append(append(testDown1, testDown2...), testDown3...), testDown4...), testDown5...)
	log.PrintFloat("Download speed", math.Quartile(downloadTests, 0.9), 2, "Mbps", log.Green)

	// Upload tests
	testUp1, err := measureUpload(11000, 10)
	if err != nil {
		return fmt.Errorf("failed to measure 11kB upload: %w", err)
	}

	testUp2, err := measureUpload(101000, 10)
	if err != nil {
		return fmt.Errorf("failed to measure 100kB upload: %w", err)
	}

	testUp3, err := measureUpload(1001000, 8)
	if err != nil {
		return fmt.Errorf("failed to measure 1MB upload: %w", err)
	}

	uploadTests := append(append(testUp1, testUp2...), testUp3...)
	log.PrintFloat("Upload speed", math.Quartile(uploadTests, 0.9), 2, "Mbps", log.Green)

	return nil
}
