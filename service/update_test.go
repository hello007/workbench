package service

import (
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name     string
		v1       string
		v2       string
		expected int
	}{
		// 相等
		{"相同版本", "1.0.0", "1.0.0", 0},
		{"相同版本带v前缀", "v1.0.0", "1.0.0", 0},
		{"都是v前缀", "v1.0.0", "v1.0.0", 0},

		// 大于
		{"主版本号大", "2.0.0", "1.0.0", 1},
		{"次版本号大", "1.1.0", "1.0.0", 1},
		{"修订号大", "1.0.9", "1.0.8", 1},
		{"实际更新场景", "1.0.9", "1.0.8", 1},

		// 小于
		{"主版本号小", "1.0.0", "2.0.0", -1},
		{"次版本号小", "1.0.0", "1.1.0", -1},
		{"修订号小", "1.0.8", "1.0.9", -1},
		{"实际当前版本旧", "1.0.7", "1.0.8", -1},

		// 不同位数
		{"两位vs三位", "1.0", "1.0.0", 0},
		{"一位vs三位", "1", "1.0.0", 0},
		{"短版本小于", "1.0", "1.0.1", -1},
		{"短版本大于", "1.1", "1.0.9", 1},

		// dev 版本
		{"dev版本", "dev", "1.0.0", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompareVersions(tt.v1, tt.v2)
			if result != tt.expected {
				t.Errorf("CompareVersions(%q, %q) = %d, expected %d", tt.v1, tt.v2, result, tt.expected)
			}
		})
	}
}

func TestFormatSpeed(t *testing.T) {
	tests := []struct {
		bytesPerSec float64
		unit        string
	}{
		{500, "B/s"},
		{1500, "KB/s"},
		{1500000, "MB/s"},
	}

	for _, tt := range tests {
		result := formatSpeed(tt.bytesPerSec)
		if !strContains(result, tt.unit) {
			t.Errorf("formatSpeed(%v) = %q, expected to contain %q", tt.bytesPerSec, result, tt.unit)
		}
	}
}

func strContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
