package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
)

// Stats utility functions
func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}

func jitter(values []float64) float64 {
	if len(values) <= 1 {
		return 0
	}
	var sum float64
	avg := average(values)
	for _, v := range values {
		sum += (v - avg) * (v - avg)
	}
	return (sum / float64(len(values)-1))
}

func quartile(values []float64, q float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	pos := int(float64(len(sorted)) * q)
	if pos >= len(sorted) {
		pos = len(sorted) - 1
	}
	return sorted[pos]
}

// HTTP client functionality
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

	return []float64{min, max, average(measurements), median(measurements), jitter(measurements)}, nil
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

func logInfo(text, data string) {
	bold := color.New(color.Bold).SprintFunc()
	blue := color.New(color.FgBlue).SprintFunc()
	padding := 15 - len(text)
	if padding < 0 {
		padding = 0
	}
	fmt.Println(bold(strings.Repeat(" ", padding), text+":", blue(data)))
}

func logLatency(data []float64) {
	bold := color.New(color.Bold).SprintFunc()
	magenta := color.New(color.FgMagenta).SprintFunc()
	fmt.Println(bold("         Latency:", magenta(fmt.Sprintf("%.2f ms", data[3]))))
	fmt.Println(bold("          Jitter:", magenta(fmt.Sprintf("%.2f ms", data[4]))))
}

func logSpeedTestResult(size string, test []float64) {
	bold := color.New(color.Bold).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	padding := 9 - len(size)
	if padding < 0 {
		padding = 0
	}
	speed := median(test)
	fmt.Println(bold(strings.Repeat(" ", padding), size, "speed:", yellow(fmt.Sprintf("%.2f Mbps", speed))))
}

func logDownloadSpeed(tests []float64) {
	bold := color.New(color.Bold).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	fmt.Println(bold("  Download speed:", green(fmt.Sprintf("%.2f Mbps", quartile(tests, 0.9)))))
}

func logUploadSpeed(tests []float64) {
	bold := color.New(color.Bold).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	fmt.Println(bold("    Upload speed:", green(fmt.Sprintf("%.2f Mbps", quartile(tests, 0.9)))))
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
	logInfo("Server location", fmt.Sprintf("%s (%s)", city, traceData["colo"]))
	logInfo("Your IP", fmt.Sprintf("%s (%s)", traceData["ip"], traceData["loc"]))

	logLatency(pingResults)

	testDown1, err := measureDownload(101000, 10)
	if err != nil {
		return fmt.Errorf("failed to measure 100kB download: %w", err)
	}
	logSpeedTestResult("100kB", testDown1)

	testDown2, err := measureDownload(1001000, 8)
	if err != nil {
		return fmt.Errorf("failed to measure 1MB download: %w", err)
	}
	logSpeedTestResult("1MB", testDown2)

	testDown3, err := measureDownload(10001000, 6)
	if err != nil {
		return fmt.Errorf("failed to measure 10MB download: %w", err)
	}
	logSpeedTestResult("10MB", testDown3)

	testDown4, err := measureDownload(25001000, 4)
	if err != nil {
		return fmt.Errorf("failed to measure 25MB download: %w", err)
	}
	logSpeedTestResult("25MB", testDown4)

	testDown5, err := measureDownload(100001000, 1)
	if err != nil {
		return fmt.Errorf("failed to measure 100MB download: %w", err)
	}
	logSpeedTestResult("100MB", testDown5)

	downloadTests := append(append(append(append(testDown1, testDown2...), testDown3...), testDown4...), testDown5...)
	logDownloadSpeed(downloadTests)

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
	logUploadSpeed(uploadTests)

	return nil
}

func main() {
	fmt.Println("Cloudflare Speed Test")
	if err := speedTest(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
