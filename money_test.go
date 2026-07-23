package prorata

import (
	"reflect"
	"testing"
)

func TestAllocate(t *testing.T) {
	tests := []struct {
		name    string
		amount  Money
		weights []int64
		want    []Money
	}{
		{"even thirds with remainder to lowest indexes", 1001, []int64{1, 1, 1}, []Money{334, 334, 333}},
		{"showcase proration 13 used / 17 remaining of $20", 2000, []int64{13, 17}, []Money{867, 1133}},
		{"negative mirrors positive", -2000, []int64{13, 17}, []Money{-867, -1133}},
		{"zero amount", 0, []int64{3, 7}, []Money{0, 0}},
		{"zero weight gets zero part", 100, []int64{0, 1}, []Money{0, 100}},
		{"single weight takes all", 555, []int64{9}, []Money{555}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.amount.Allocate(tt.weights)
			if err != nil {
				t.Fatalf("Allocate(%v) error: %v", tt.weights, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Allocate(%v) = %v, want %v", tt.weights, got, tt.want)
			}
		})
	}
}

func TestAllocateErrors(t *testing.T) {
	tests := []struct {
		name    string
		weights []int64
	}{
		{"no weights", nil},
		{"negative weight", []int64{5, -1}},
		{"zero total weight", []int64{0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Money(100).Allocate(tt.weights); err == nil {
				t.Fatalf("Allocate(%v) expected error, got nil", tt.weights)
			}
		})
	}
}

func TestPercent(t *testing.T) {
	tests := []struct {
		name   string
		amount Money
		bps    int64
		want   Money
	}{
		{"20% of $20.00", 2000, 2000, 400},
		{"half rounds away from zero", 15, 1000, 2},
		{"negative half rounds away from zero", -15, 1000, -2},
		{"below half rounds down", 125, 100, 1},
		{"100%", 480, 10000, 480},
		{"0%", 480, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.amount.Percent(tt.bps); got != tt.want {
				t.Fatalf("Money(%d).Percent(%d) = %d, want %d", tt.amount, tt.bps, got, tt.want)
			}
		})
	}
}

func TestMoneyString(t *testing.T) {
	tests := []struct {
		amount Money
		want   string
	}{
		{2000, "20.00"},
		{-5, "-0.05"},
		{0, "0.00"},
		{-37267, "-372.67"},
	}
	for _, tt := range tests {
		if got := tt.amount.String(); got != tt.want {
			t.Fatalf("Money(%d).String() = %q, want %q", tt.amount, got, tt.want)
		}
	}
}
