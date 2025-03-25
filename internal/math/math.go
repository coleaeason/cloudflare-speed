package math

import "sort"

// Average calculates the arithmetic mean of a slice of float64 values
func Average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// Median calculates the median value of a slice of float64 values
func Median(values []float64) float64 {
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

// Jitter calculates the variance of a slice of float64 values
func Jitter(values []float64) float64 {
	if len(values) <= 1 {
		return 0
	}
	var sum float64
	avg := Average(values)
	for _, v := range values {
		sum += (v - avg) * (v - avg)
	}
	return (sum / float64(len(values)-1))
}

// Quartile finds the value at a specified quartile in a slice of float64 values
func Quartile(values []float64, q float64) float64 {
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
