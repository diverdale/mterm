package config

import (
	"reflect"
	"testing"
)

func TestGroupSegments(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"Home", []string{"Home"}},
		{"Home/Media", []string{"Home", "Media"}},
		{"/Home/Media/", []string{"Home", "Media"}},
		{"Home//Media", []string{"Home", "Media"}},
		{"  Home  /  Media  ", []string{"Home", "Media"}},
		{"a/b/c/d", []string{"a", "b", "c", "d"}},
	}
	for _, tc := range cases {
		got := GroupSegments(tc.in)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("GroupSegments(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestCommonPrefixLen(t *testing.T) {
	cases := []struct {
		a, b []string
		want int
	}{
		{nil, nil, 0},
		{[]string{"x"}, nil, 0},
		{[]string{"Home"}, []string{"Home"}, 1},
		{[]string{"Home", "Media"}, []string{"Home", "Dev"}, 1},
		{[]string{"Home", "Media"}, []string{"Home", "Media"}, 2},
		{[]string{"Home", "Media"}, []string{"Work", "Media"}, 0},
		{[]string{"A", "B", "C"}, []string{"A", "B"}, 2},
	}
	for _, tc := range cases {
		if got := CommonPrefixLen(tc.a, tc.b); got != tc.want {
			t.Errorf("CommonPrefixLen(%v, %v) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
