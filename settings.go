package main

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// parseSettings reads a comma separated list of "key:value" pairs - "style:plain,
// justify:right" - passing each value to the reader its key names.
//
// An item that is not a pair, a key no reader is named for, or a value its reader refuses,
// ends the reading and is returned as the error.
func parseSettings(given string, readers map[string]func(value string) error) error {
	for _, item := range strings.Split(given, ",") {
		if item == "" {
			continue
		}
		key, value, isPair := strings.Cut(item, ":")
		if !isPair {
			return fmt.Errorf("'%s' is not a setting; expected key:value", item)
		}
		read, known := readers[key]
		if !known {
			return fmt.Errorf("'%s' is not a setting; expected one of %s",
				key, strings.Join(slices.Sorted(maps.Keys(readers)), ", "))
		}
		if err := read(value); err != nil {
			return err
		}
	}
	return nil
}

// settingFrom returns the reader of a setting that takes one of allowed, writing what it
// reads to target.
func settingFrom(name string, allowed []string, target *string) func(string) error {
	return func(value string) error {
		if !slices.Contains(allowed, value) {
			return fmt.Errorf("'%s' is not %s; expected one of %s", value, name, strings.Join(allowed, ", "))
		}
		*target = value
		return nil
	}
}
