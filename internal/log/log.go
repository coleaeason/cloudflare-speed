package log

import (
	"fmt"

	"github.com/fatih/color"
)

// Available text styles
var (
	Bold    = color.New(color.Bold).SprintFunc()
	Blue    = color.New(color.FgBlue).SprintFunc()
	Green   = color.New(color.FgGreen).SprintFunc()
	Yellow  = color.New(color.FgYellow).SprintFunc()
	Magenta = color.New(color.FgMagenta).SprintFunc()
	Red     = color.New(color.FgRed).SprintFunc()
)

// Print formats and prints a message with the given colorFunc for highlighted text
func Print(prefix, format string, colorFunc func(...interface{}) string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Println(Bold(prefix, colorFunc(msg)))
}

// PrintPair prints a key-value pair with the key in bold and value in the specified color
func PrintPair(key, value string, colorFunc func(...interface{}) string) {
	fmt.Println(Bold(key+": ", colorFunc(value)))
}

// PrintValue prints a simple value with the given color
func PrintValue(label string, value interface{}, colorFunc func(...interface{}) string) {
	fmt.Println(Bold(label+": ", colorFunc(fmt.Sprintf("%v", value))))
}

// PrintFloat prints a float value with the given precision and unit
func PrintFloat(label string, value float64, precision int, unit string, colorFunc func(...interface{}) string) {
	format := fmt.Sprintf("%%.%df %s", precision, unit)
	formatted := fmt.Sprintf(format, value)
	fmt.Println(Bold(label+": ", colorFunc(formatted)))
}
