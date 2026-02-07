// Package bytesize provides utilities for parsing and formatting byte sizes.
package bytesize

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		// Basic units
		{name: "bytes lowercase", input: "100b", want: 100},
		{name: "bytes uppercase", input: "100B", want: 100},
		{name: "kilobytes lowercase", input: "1kb", want: 1024},
		{name: "kilobytes uppercase", input: "1KB", want: 1024},
		{name: "megabytes lowercase", input: "10mb", want: 10 * 1024 * 1024},
		{name: "megabytes uppercase", input: "10MB", want: 10 * 1024 * 1024},
		{name: "gigabytes lowercase", input: "1gb", want: 1024 * 1024 * 1024},
		{name: "gigabytes uppercase", input: "1GB", want: 1024 * 1024 * 1024},
		{name: "terabytes", input: "1TB", want: 1024 * 1024 * 1024 * 1024},

		// No unit defaults to bytes
		{name: "no unit", input: "1024", want: 1024},

		// Decimals
		{name: "decimal megabytes", input: "1.5MB", want: int64(1.5 * 1024 * 1024)},
		{name: "decimal gigabytes", input: "0.5GB", want: int64(0.5 * 1024 * 1024 * 1024)},

		// Whitespace handling
		{name: "space before unit", input: "100 MB", want: 100 * 1024 * 1024},
		{name: "leading whitespace", input: "  50KB", want: 50 * 1024},
		{name: "trailing whitespace", input: "50KB  ", want: 50 * 1024},

		// Edge cases
		{name: "zero", input: "0", want: 0},
		{name: "zero with unit", input: "0MB", want: 0},

		// Error cases
		{name: "empty string", input: "", wantErr: true},
		{name: "invalid unit", input: "100XB", wantErr: true},
		{name: "negative value", input: "-100MB", wantErr: true},
		{name: "just letters", input: "MB", wantErr: true},
		{name: "invalid number", input: "abc", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFormat(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{name: "zero", bytes: 0, want: "0 B"},
		{name: "bytes", bytes: 100, want: "100 B"},
		{name: "kilobytes", bytes: 1024, want: "1.00 KB"},
		{name: "kilobytes decimal", bytes: 1536, want: "1.50 KB"},
		{name: "megabytes", bytes: 1024 * 1024, want: "1.00 MB"},
		{name: "megabytes large", bytes: 100 * 1024 * 1024, want: "100.00 MB"},
		{name: "gigabytes", bytes: 1024 * 1024 * 1024, want: "1.00 GB"},
		{name: "terabytes", bytes: 1024 * 1024 * 1024 * 1024, want: "1.00 TB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Format(tt.bytes)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseRate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64 // bytes per second
		wantErr bool
	}{
		// Bits per second (common for network speeds)
		{name: "megabits", input: "10mbps", want: 10 * 1000 * 1000 / 8},
		{name: "megabits uppercase", input: "10Mbps", want: 10 * 1000 * 1000 / 8},
		{name: "gigabits", input: "1gbps", want: 1000 * 1000 * 1000 / 8},
		{name: "kilobits", input: "100kbps", want: 100 * 1000 / 8},

		// Bytes per second
		{name: "megabytes per sec", input: "10MB/s", want: 10 * 1024 * 1024},
		{name: "kilobytes per sec", input: "100KB/s", want: 100 * 1024},
		{name: "gigabytes per sec", input: "1GB/s", want: 1024 * 1024 * 1024},

		// Error cases
		{name: "empty", input: "", wantErr: true},
		{name: "invalid", input: "fast", wantErr: true},
		{name: "negative", input: "-10mbps", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRate(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFormatRate(t *testing.T) {
	tests := []struct {
		name        string
		bytesPerSec int64
		want        string
	}{
		{name: "zero", bytesPerSec: 0, want: "0 bps"},
		{name: "kilobits", bytesPerSec: 125, want: "1.00 Kbps"},    // 1000 bits = 125 bytes
		{name: "megabits", bytesPerSec: 125000, want: "1.00 Mbps"}, // 1M bits = 125KB
		{name: "10 megabits", bytesPerSec: 1250000, want: "10.00 Mbps"},
		{name: "gigabit", bytesPerSec: 125000000, want: "1.00 Gbps"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatRate(tt.bytesPerSec)
			assert.Equal(t, tt.want, got)
		})
	}
}
