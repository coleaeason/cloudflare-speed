package log

import (
	"fmt"

	"github.com/fatih/color"
)

// LogInfo prints general information with blue color highlights
func Info(text, data string) {
	bold := color.New(color.Bold).SprintFunc()
	blue := color.New(color.FgBlue).SprintFunc()
	fmt.Println(bold("\t", text+": ", blue(data)))
}

// LogLatency prints latency and jitter information with magenta color highlights
func Latency(data []float64) {
	bold := color.New(color.Bold).SprintFunc()
	magenta := color.New(color.FgMagenta).SprintFunc()
	fmt.Println(bold("\tLatency: ", magenta(fmt.Sprintf("%.2f ms", data[3]))))
	fmt.Println(bold("\tJitter:  ", magenta(fmt.Sprintf("%.2f ms", data[4]))))
}

// LogSpeedTestResult prints individual speed test results with yellow color highlights
func SpeedTestResult(size string, test []float64, medianFn func([]float64) float64) {
	bold := color.New(color.Bold).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	speed := medianFn(test)
	fmt.Println(bold("\t", size, "speed: ", yellow(fmt.Sprintf("%.2f Mbps", speed))))
}

// LogDownloadSpeed prints overall download speed with green color highlights
func DownloadSpeed(tests []float64, quartileFn func([]float64, float64) float64) {
	bold := color.New(color.Bold).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	fmt.Println(bold("\tDownload speed: ", green(fmt.Sprintf("%.2f Mbps", quartileFn(tests, 0.9)))))
}

// LogUploadSpeed prints overall upload speed with green color highlights
func UploadSpeed(tests []float64, quartileFn func([]float64, float64) float64) {
	bold := color.New(color.Bold).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	fmt.Println(bold("\tUpload speed: ", green(fmt.Sprintf("%.2f Mbps", quartileFn(tests, 0.9)))))
}
