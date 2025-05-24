package handler

import (
	"strconv"
	"strings"
)

func parseCommaSeparatedInts(s string) []int {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var result []int
	for _, part := range parts {
		if num, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
			result = append(result, num)
		}
	}
	return result
}

func parsePositiveInt(s string, defaultVal int) int {
	if val, err := strconv.Atoi(s); err == nil && val > 0 {
		return val
	}
	return defaultVal
}
