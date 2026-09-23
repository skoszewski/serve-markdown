package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeConfig writes a configuration file into directory and returns its path.
func writeConfig(t *testing.T, directory, name, content string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	writeFile(t, path, content)
	return path
}

// userConfigDir points the user's configuration directory at an empty directory of its own and
// returns it.
func userConfigDir(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for _, variable := range []string{"HOME", "XDG_CONFIG_HOME", "AppData", "home"} {
		t.Setenv(variable, home)
	}
	directory, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	return directory
}

func TestReadConfig(t *testing.T) {
	root := documentRoot(t)
	userConfigDir(t)
	t.Chdir(root)
	writeConfig(t, root, configName, "index-only: true\nlist: true\nport: 9000\nseparators: hidden\n"+
		"outline:\n  style: numbered-hierarchical\n  justify: right\n")

	config, name, err := readConfigFile("")
	if err != nil {
		t.Fatal(err)
	}
	if name != configName {
		t.Errorf("name = %q, want the file in the directory the server was started in", name)
	}
	if config.IndexOnly == nil || !*config.IndexOnly {
		t.Errorf("index-only = %v, want true", config.IndexOnly)
	}
	if config.Port == nil || *config.Port != 9000 {
		t.Errorf("port = %v, want 9000", config.Port)
	}
	if config.Separators == nil || *config.Separators != "hidden" {
		t.Errorf("separators = %v, want hidden", config.Separators)
	}
	if config.List == nil || !config.List.on || len(config.List.settings) != 0 {
		t.Errorf("list = %+v, want it drawn with its own settings", config.List)
	}
	if config.Outline == nil || config.Outline.settings["justify"] != "right" {
		t.Errorf("outline = %+v, want the settings the file names", config.Outline)
	}
	// A key the file says nothing about leaves the command line to say it.
	if config.Mermaid != nil || config.ListenAddress != nil {
		t.Errorf("config = %+v, want nothing where the file is silent", config)
	}
}

func TestReadConfigLooksForTheFileInOrder(t *testing.T) {
	root := documentRoot(t)
	started := filepath.Join(root, "started")
	user := filepath.Join(userConfigDir(t), userConfigName)
	named := writeConfig(t, root, "named.yaml", "port: 1\n")
	if err := os.Mkdir(started, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(started)

	// Without a file anywhere everything is left to the command line, and quietly.
	config, name, err := readConfigFile("")
	if err != nil || name != "" || config.Port != nil {
		t.Errorf("readConfigFile() = %+v, %q, %v; want nothing at all", config, name, err)
	}

	type step struct {
		write, named, want string
	}
	steps := []step{{user, "", user}}
	if runtime.GOOS == "darwin" {
		dotConfig := filepath.Join(os.Getenv("HOME"), ".config", userConfigName)
		steps = append(steps, step{dotConfig, "", dotConfig})
	}
	steps = append(steps, step{filepath.Join(started, configName), "", configName},
		step{"", named, named})
	for _, step := range steps {
		if step.write != "" {
			writeConfig(t, filepath.Dir(step.write), filepath.Base(step.write), "port: 2\n")
		}
		config, name, err := readConfigFile(step.named)
		if err != nil {
			t.Fatal(err)
		}
		if name != step.want || config.Port == nil {
			t.Errorf("readConfigFile(%q) read %q, want %q", step.named, name, step.want)
		}
	}

	// A file named on the command line must be there.
	if _, _, err := readConfigFile(filepath.Join(root, "nowhere.yaml")); err == nil {
		t.Error("a file that is not there was read")
	}
}

func TestReadConfigRefusesWhatItCannotRead(t *testing.T) {
	root := documentRoot(t)

	tests := map[string]string{
		"outlyne: true\n":            "'outlyne' is not a setting",
		"outline: sideways\n":        "expected true, false or a mapping of settings",
		"port: many\n":               "cannot unmarshal",
		"outline:\n  - style: nh\n":  "expected true, false or a mapping of settings",
		"index-only: true\n  bad:\n": "yaml",
	}
	for content, want := range tests {
		t.Run(content, func(t *testing.T) {
			name := writeConfig(t, root, "given.yaml", content)
			_, _, err := readConfigFile(name)
			if err == nil {
				t.Fatalf("readConfigFile(%q) raised no error", content)
			}
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error = %q, want one holding %q", err, want)
			}
		})
	}
}

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

func TestReadConfigSearch(t *testing.T) {
	root := documentRoot(t)

	// A file writes the documents a folder is read as either way it writes a list.
	tests := map[string][]string{
		"search:\n  - index.md\n  - HOME.md\n": {"index.md", "HOME.md"},
		"search: index.md, HOME.md\n":          {"index.md", "HOME.md"},
		"search: HOME.md\n":                    {"HOME.md"},
	}
	for content, want := range tests {
		t.Run(content, func(t *testing.T) {
			name := writeConfig(t, root, "given.yaml", content)
			config, _, err := readConfigFile(name)
			if err != nil {
				t.Fatal(err)
			}
			if config.Search == nil || strings.Join(config.Search.names, ",") != strings.Join(want, ",") {
				t.Errorf("search = %+v, want %v", config.Search, want)
			}
		})
	}

	// A mapping is no list of names.
	name := writeConfig(t, root, "given.yaml", "search:\n  first: index.md\n")
	if _, _, err := readConfigFile(name); err == nil {
		t.Error("a mapping was read as a list of names")
	}
}

func TestSplitNames(t *testing.T) {
	tests := map[string][]string{
		"README.md,index.md":    {"README.md", "index.md"},
		" README.md , index.md": {"README.md", "index.md"},
		"README.md,,":           {"README.md"},
		"":                      nil,
		",":                     nil,
	}
	for given, want := range tests {
		t.Run(given, func(t *testing.T) {
			if got := splitNames(given); strings.Join(got, ",") != strings.Join(want, ",") {
				t.Errorf("splitNames(%q) = %v, want %v", given, got, want)
			}
		})
	}
}

func TestBodyClass(t *testing.T) {
	outline := sidebars{Outline: outlineSettings{Style: "plain", Justify: "right"}}

	tests := map[string]struct {
		settings pageSettings
		want     string
	}{
		// The window needs no rule capping it, so it carries no class of its own.
		"the width it is given unasked": {pageSettings{ContentWidth: defaultContentWidth}, ""},
		"a width nobody set":            {pageSettings{}, ""},
		"a capped width":                {pageSettings{ContentWidth: "medium"}, "width-medium"},
		"a capped width beside an outline": {pageSettings{ContentWidth: "small", Sidebars: outline},
			"width-small with-sidebar outline-right"},
		"the window beside an outline": {pageSettings{ContentWidth: widthFull, Sidebars: outline},
			"with-sidebar outline-right"},
		"no separators":           {pageSettings{Separators: "hidden"}, "separators-hidden"},
		"the top separator alone": {pageSettings{Separators: "top"}, "separators-top"},
		"the side separators":     {pageSettings{Separators: "side"}, "separators-side"},
		"every separator":         {pageSettings{Separators: "all"}, ""},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := test.settings.bodyClass(); got != test.want {
				t.Errorf("bodyClass = %q, want %q", got, test.want)
			}
		})
	}
}

func TestWatchInterval(t *testing.T) {
	// A page reading Azure Repos checks for nothing, whatever seconds were asked for; a local
	// page checks at the seconds it was given, or at the default.
	tests := []struct {
		settings configuration
		kind     sourceKind
		want     float64
	}{
		{configuration{}, kindLocal, defaultWatchInterval},
		{configuration{}, kindADO, 0},
		{configuration{watch: 0.5}, kindLocal, 0.5},
		{configuration{watch: 0.5}, kindADO, 0},
	}
	for _, test := range tests {
		if got := test.settings.watchInterval(test.kind); got != test.want {
			t.Errorf("watchInterval(%v) with %+v = %g, want %g",
				test.kind, test.settings, got, test.want)
		}
	}
}

func TestChooseSettings(t *testing.T) {
	off := outlineSettings{Justify: "left", Display: "shown"}

	tests := map[string]struct {
		given         string
		onCommandLine bool
		fromFile      *settingsValue
		want          outlineSettings
	}{
		"neither says anything": {"", false, nil, off},
		"the command line alone": {"style:nh", true,
			nil, outlineSettings{Style: "numbered-hierarchical", Justify: "left", Display: "shown"}},
		"the file alone": {"", false,
			&settingsValue{on: true, settings: map[string]string{"style": "plain", "display": "hidden"}},
			outlineSettings{Style: "plain", Justify: "left", Display: "hidden"}},
		"the file drawing it with its own settings": {"", false,
			&settingsValue{on: true}, defaultOutline},
		"the file turning it off": {"", false, &settingsValue{}, off},
		// The command line stands above the file, written or written empty.
		"the command line over the file": {"style:numbered", true,
			&settingsValue{on: true, settings: map[string]string{"style": "plain"}},
			outlineSettings{Style: "numbered", Justify: "left", Display: "shown"}},
		"the command line turning it off": {"", true,
			&settingsValue{on: true, settings: map[string]string{"style": "plain"}}, off},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := chooseSettings(test.given, test.onCommandLine, test.fromFile,
				off, defaultOutline, parseOutline)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Errorf("settings = %+v, want %+v", got, test.want)
			}
		})
	}

	// A setting a file names is refused for the same reasons the command line's is.
	asked := &settingsValue{on: true, settings: map[string]string{"scope": "current"}}
	if _, err := chooseSettings("", false, asked, off, defaultOutline, parseOutline); err == nil {
		t.Error("a setting the flag does not know was read from a file")
	}
}
