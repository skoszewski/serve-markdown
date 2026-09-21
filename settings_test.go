package main

import (
	"strings"
	"testing"
)

func TestParseSettings(t *testing.T) {
	var colour, size string
	readers := map[string]func(string) error{
		"colour": settingFrom("a colour", []string{"red", "green"}, &colour),
		"size":   settingFrom("a size", []string{"small", "large"}, &size),
	}

	if err := parseSettings("colour:red,size:large", readers); err != nil {
		t.Fatal(err)
	}
	if colour != "red" || size != "large" {
		t.Errorf("colour, size = %q, %q, want \"red\", \"large\"", colour, size)
	}

	if err := parseSettings("size:small", readers); err != nil {
		t.Fatal(err)
	}
	if colour != "red" || size != "small" {
		t.Errorf("colour, size = %q, %q, want \"red\", \"small\"", colour, size)
	}

	if err := parseSettings("", readers); err != nil {
		t.Errorf("an empty list raised %v", err)
	}

	tests := map[string]string{
		"colour":       "expected key:value",
		"shape:round":  "expected one of colour, size",
		"colour:mauve": "expected one of red, green",
	}
	for given, want := range tests {
		t.Run(given, func(t *testing.T) {
			err := parseSettings(given, readers)
			if err == nil {
				t.Fatalf("parseSettings(%q) raised no error", given)
			}
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error = %q, want one holding %q", err, want)
			}
		})
	}
}
