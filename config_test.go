package main

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/skoszewski/serve-markdown/v3/render"
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
	off := render.OutlineSettings{Justify: "left", Display: "shown"}

	tests := map[string]struct {
		given         string
		onCommandLine bool
		fromFile      *settingsValue
		want          render.OutlineSettings
	}{
		"neither says anything": {"", false, nil, off},
		"the command line alone": {"style:nh", true,
			nil, render.OutlineSettings{Style: "numbered-hierarchical", Justify: "left", Display: "shown"}},
		"the file alone": {"", false,
			&settingsValue{on: true, settings: map[string]string{"style": "plain", "display": "hidden"}},
			render.OutlineSettings{Style: "plain", Justify: "left", Display: "hidden"}},
		"the file drawing it with its own settings": {"", false,
			&settingsValue{on: true}, render.DefaultOutline},
		"the file turning it off": {"", false, &settingsValue{}, off},
		// The command line stands above the file, written or written empty.
		"the command line over the file": {"style:numbered", true,
			&settingsValue{on: true, settings: map[string]string{"style": "plain"}},
			render.OutlineSettings{Style: "numbered", Justify: "left", Display: "shown"}},
		"the command line turning it off": {"", true,
			&settingsValue{on: true, settings: map[string]string{"style": "plain"}}, off},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := chooseSettings(test.given, test.onCommandLine, test.fromFile,
				off, render.DefaultOutline, parseOutline)
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
	if _, err := chooseSettings("", false, asked, off, render.DefaultOutline, parseOutline); err == nil {
		t.Error("a setting the flag does not know was read from a file")
	}

	// A list written in a file takes its display the way the outline does.
	fromFile := &settingsValue{on: true, settings: map[string]string{"display": "hidden"}}
	list, err := chooseSettings("", false, fromFile, render.ListSettings{Display: render.DefaultList.Display},
		render.DefaultList, parseList)
	if err != nil {
		t.Fatal(err)
	}
	if want := (render.ListSettings{Scope: "current", Display: "hidden"}); list != want {
		t.Errorf("list = %+v, want %+v", list, want)
	}
}

func TestPDFPageFor(t *testing.T) {
	// Nothing said leaves the default page.
	page, err := pdfPageFor(nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if page != defaultPDFPage {
		t.Errorf("page = %+v, want %+v", page, defaultPDFPage)
	}

	// Each setting is taken from the last of the file, the document and the command line that
	// names it.
	fromFile := &settingsValue{on: true, settings: map[string]string{"paper": "letter",
		"footer": "page", "header": "title"}}
	fromDocument := map[string]string{"paper": "a5", "footer": "none"}
	if page, err = pdfPageFor(fromFile, fromDocument, "paper:a3"); err != nil {
		t.Fatal(err)
	}
	want := defaultPDFPage
	want.Paper, want.Footer, want.Header = "a3", "none", "title"
	if page != want {
		t.Errorf("page = %+v, want %+v", page, want)
	}

	// A file turning the setting off says nothing.
	if page, err = pdfPageFor(&settingsValue{}, nil, ""); err != nil || page != defaultPDFPage {
		t.Errorf("page = %+v (%v), want %+v", page, err, defaultPDFPage)
	}

	// A setting no layer may say is refused, wherever it is written.
	for _, test := range []struct {
		fromFile     *settingsValue
		fromDocument map[string]string
		flag         string
	}{
		{&settingsValue{on: true, settings: map[string]string{"paper": "b5"}}, nil, ""},
		{nil, map[string]string{"margin": "5em"}, ""},
		{nil, nil, "footer:total"},
	} {
		if _, err := pdfPageFor(test.fromFile, test.fromDocument, test.flag); err == nil {
			t.Errorf("pdfPageFor(%+v, %v, %q) raised no error", test.fromFile, test.fromDocument, test.flag)
		}
	}
}

func TestEditorConfigFlagNeedsTheEditor(t *testing.T) {
	root := documentRoot(t)
	userConfigDir(t)
	t.Chdir(root)
	writeConfig(t, root, configName, "editor-config: custom.json\n")

	command := flag.CommandLine
	defer func() { flag.CommandLine = command }()
	flag.CommandLine = flag.NewFlagSet("serve-markdown", flag.ContinueOnError)
	arguments := os.Args
	defer func() { os.Args = arguments }()

	os.Args = []string{"serve-markdown"}
	if _, err := readConfiguration(); err == nil || !strings.Contains(err.Error(), "--editor-config needs --editor") {
		t.Errorf("error = %v, want one saying --editor-config needs --editor", err)
	}

	flag.CommandLine = flag.NewFlagSet("serve-markdown", flag.ContinueOnError)
	os.Args = []string{"serve-markdown", "--editor"}
	settings, err := readConfiguration()
	if err != nil {
		t.Fatal(err)
	}
	if settings.editorConfig != "custom.json" {
		t.Errorf("editorConfig = %q, want the file's", settings.editorConfig)
	}
}

func TestReadConfigMenu(t *testing.T) {
	root := documentRoot(t)
	name := writeConfig(t, root, "menu.yaml", "menu:\n  - Home: README.md\n  - Guides:\n"+
		"      - Guide: /docs/guide.md\n      - Deeper:\n          - docs/index.md\n")
	config, _, err := readConfigFile(name)
	if err != nil {
		t.Fatal(err)
	}
	var flatten func(entries []render.MenuEntry, prefix string) []string
	flatten = func(entries []render.MenuEntry, prefix string) []string {
		var names []string
		for _, entry := range entries {
			names = append(names, prefix+entry.Label+"="+entry.Path)
			names = append(names, flatten(entry.Entries, prefix+entry.Label+"/")...)
		}
		return names
	}
	want := "Home=README.md Guides= Guides/Guide=docs/guide.md Guides/Deeper= Guides/Deeper/=docs/index.md"
	if got := strings.Join(flatten(config.Menu, ""), " "); got != want {
		t.Errorf("menu = %q, want %q", got, want)
	}

	for content, want := range map[string]string{
		"menu:\n  - Home: README.md\n    Other: notes.md\n": "line 2: expected a menu entry",
		"menu:\n  - Home: 3\n":                              "line 2: 'Home' needs a document path",
		"menu:\n  - Guides: []\n":                           "line 2: 'Guides' needs a document path",
		"menu:\n  - Home:\n":                                "line 2: 'Home' needs a document path",
		"menu:\n  - 42\n":                                   "line 2: expected a menu entry",
		"menu:\n  - '': README.md\n":                        "line 2: a menu entry needs a label",
	} {
		name := writeConfig(t, root, "bad.yaml", content)
		if _, _, err := readConfigFile(name); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error = %v, want one holding %q", content, err, want)
		}
	}
}

func TestReadConfigReadsTheMenuOfTheStartingDirectoryAlone(t *testing.T) {
	root := documentRoot(t)
	user := userConfigDir(t)
	t.Chdir(root)
	menu := "list: true\nmenu:\n  - Home: README.md\n"
	writeConfig(t, filepath.Join(user, filepath.Dir(userConfigName)), filepath.Base(userConfigName), menu)

	command := flag.CommandLine
	defer func() { flag.CommandLine = command }()
	arguments := os.Args
	defer func() { os.Args = arguments }()

	// The user's own file is read for every directory, so its menu is not used.
	flag.CommandLine = flag.NewFlagSet("serve-markdown", flag.ContinueOnError)
	os.Args = []string{"serve-markdown"}
	settings, err := readConfiguration()
	if err != nil {
		t.Fatal(err)
	}
	if settings.menu != nil || settings.menuFile != "" || !strings.Contains(settings.menuWarning, "is ignored") ||
		!settings.listGiven {
		t.Errorf("menu = %v, file = %q, warning = %q, listGiven = %v; want no menu, a warning, and the list given",
			settings.menu, settings.menuFile, settings.menuWarning, settings.listGiven)
	}

	// The file in the starting directory, and one --config names, keep theirs.
	writeConfig(t, root, configName, "menu:\n  - Home: README.md\n")
	named := writeConfig(t, root, "named.yaml", "menu:\n  - Home: README.md\n")
	for _, args := range [][]string{{"serve-markdown"}, {"serve-markdown", "--config", named}} {
		flag.CommandLine = flag.NewFlagSet("serve-markdown", flag.ContinueOnError)
		os.Args = args
		if settings, err = readConfiguration(); err != nil {
			t.Fatal(err)
		}
		if len(settings.menu) != 1 || settings.menuFile != settings.file || settings.menuWarning != "" || settings.listGiven {
			t.Errorf("%q: menu = %v, file = %q, warning = %q, listGiven = %v; want the menu alone, read from its file",
				args, settings.menu, settings.menuFile, settings.menuWarning, settings.listGiven)
		}
	}
}

func TestReadConfigIgnoreFileAndEditorExclude(t *testing.T) {
	root := documentRoot(t)
	userConfigDir(t)
	t.Chdir(root)
	writeConfig(t, root, configName, "ignore-file: from-file.txt\neditor-exclude: [\"**/drafts\", \"!keep.md\"]\n")

	command := flag.CommandLine
	defer func() { flag.CommandLine = command }()
	arguments := os.Args
	defer func() { os.Args = arguments }()

	flag.CommandLine = flag.NewFlagSet("serve-markdown", flag.ContinueOnError)
	os.Args = []string{"serve-markdown"}
	settings, err := readConfiguration()
	if err != nil {
		t.Fatal(err)
	}
	if settings.ignoreFile != "from-file.txt" || strings.Join(settings.editorExclude, ",") != "**/drafts,!keep.md" {
		t.Errorf("ignoreFile = %q, editorExclude = %q, want the file's", settings.ignoreFile, settings.editorExclude)
	}

	flag.CommandLine = flag.NewFlagSet("serve-markdown", flag.ContinueOnError)
	os.Args = []string{"serve-markdown", "--ignore-file", "from-flag.txt"}
	if settings, err = readConfiguration(); err != nil {
		t.Fatal(err)
	}
	if settings.ignoreFile != "from-flag.txt" {
		t.Errorf("ignoreFile = %q, want the flag's", settings.ignoreFile)
	}

	flag.CommandLine = flag.NewFlagSet("serve-markdown", flag.ContinueOnError)
	os.Args = []string{"serve-markdown", "--editor-exclude", "[a"}
	if _, err := readConfiguration(); err == nil || !strings.Contains(err.Error(), "editor-exclude") {
		t.Errorf("error = %v, want one naming editor-exclude", err)
	}
}
