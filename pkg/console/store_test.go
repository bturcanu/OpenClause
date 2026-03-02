package console

import "testing"

func TestClampLimit(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{0, 100},
		{-1, 100},
		{50, 50},
		{100, 100},
		{200, 100},
		{1, 1},
	}
	for _, tt := range tests {
		got := clampLimit(tt.input)
		if got != tt.want {
			t.Errorf("clampLimit(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestClampOffset(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{-5, 0},
		{-1, 0},
		{0, 0},
		{10, 10},
		{999, 999},
	}
	for _, tt := range tests {
		got := clampOffset(tt.input)
		if got != tt.want {
			t.Errorf("clampOffset(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
