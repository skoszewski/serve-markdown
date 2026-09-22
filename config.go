package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// This file is where the server is told what to do: the settings it knows, what each is
// without being told, how they are written on the command line and in a file, and which of
// the two is believed. Nothing else reads a flag or a file to find out.

// configName is the file read as the configuration when --config names none.
const configName = "serve-markdown.yaml"

// What the server does when it is told nothing.
const (
	defaultListenAddress = "127.0.0.1"
	defaultServePort     = 8000
	defaultWatchInterval = 1.0
	defaultContentWidth  = "full"
)

// defaultSearch names the documents a folder is read as, in the order they are looked for.
var defaultSearch = []string{"README.md", "index.md"}

// widthFull is the width that caps the document at nothing, giving it the window.
const widthFull = "full"

// contentWidths are the widths --content-width takes, the styling holding what each measures:
// 780px of prose, the 980px github-markdown-css is written for, the 1280px a 1080p screen
// leaves beside both sidebars, and the window itself, which is what a page is given unasked.
var contentWidths = []string{"small", "medium", "large", widthFull}

// defaultOutline is what an outline is drawn with before --outline or a page names anything
// else, and defaultList the same for a directory list.
var (
	defaultOutline = outlineSettings{Style: "plain", Justify: "left"}
	defaultList    = listSettings{Scope: "current"}
)

// outlineStyles are the styles --outline takes, each with the shorthand that also names it:
// entries without numbers, entries numbered within each level, and entries numbered as 1.,
// 1.1., 1.1.1. down the levels.
var outlineStyles = []struct{ name, shorthand string }{
	{"plain", "p"},
	{"numbered", "n"},
	{"numbered-hierarchical", "nh"},
}

// outlineNone is the style that asks for no outline, and the scope that asks for no list.
const outlineNone = "none"

// outlineJustifications are the sides of the document the outline stands on, and listScopes
// how much of the source the directory list reaches.
var (
	outlineJustifications = []string{"left", "right"}
	listScopes            = []string{"current", "subfolders", "tree"}
)

const pathUsage = "Markdown file or directory to serve; defaults to the current directory. " +
	"A directory is served at its own URL path, resolving to the first document of --search " +
	"within it and listing its Markdown files when it holds none of them. An " +
	"'ado://<organization>/<project>/<repository>/<path to file>' URL serves a file from an " +
	"Azure Repos Git repository, under its own /_/ado/ route rather than at the root. " +
	"Whichever is given, the /_/ado/ routes reach a repository while the server runs."

// configuration is everything the server was told, the command line and the file read into
// one answer.
//
// watch is the seconds asked for between the page's checks for changes, zero leaving the
// source to say; file is the configuration file that was read, empty when there was none.
type configuration struct {
	path          string
	listenAddress string
	port          int
	online        bool
	mermaid       bool
	indexOnly     bool
	contentWidth  string
	search        indexSearch
	outline       outlineSettings
	list          listSettings
	ado           adoSettings
	showVersion   bool
	watch         float64
	file          string
}

// watchInterval returns the seconds between a page's checks for changes to its document, for
// a source of the kind given.
func (c configuration) watchInterval(kind sourceKind) float64 {
	local := defaultWatchInterval
	if c.watch > 0 {
		local = c.watch
	}
	return watchIntervalFor(kind, local)
}

// watchIntervalFor returns the seconds a page reading a source of this kind checks for
// changes at, local being the seconds a local page checks at.
//
// A page reading Azure Repos checks for none: the document is read over the network, and a
// repository changes when someone pushes to it rather than while it is being written, so the
// browser's own refresh is what reads it again.
func watchIntervalFor(kind sourceKind, local float64) float64 {
	if kind == kindADO {
		return 0
	}
	return local
}

// readConfiguration reads the command line and the configuration file into what the server is
// to do.
//
// A flag written on the command line stands above what the file says; a setting neither names
// is what this file says it is.
func readConfiguration() (configuration, error) {
	listenAddress := flag.String("listen-address", defaultListenAddress,
		"Address for the local web server to listen on")
	port := flag.Int("port", defaultServePort, "Port for the local web server")
	watchInterval := flag.Float64("watch-interval", 0, fmt.Sprintf(
		"Seconds between checks for changes to the file, polled by the browser page (default "+
			"%g). A page reading %s checks for none, the browser's own refresh reading it again",
		defaultWatchInterval, adoScheme))
	online := flag.Bool("online", false,
		"Load the Markdown and highlighting libraries from their CDNs instead of from inside this binary")
	outline := flag.String("outline", "", fmt.Sprintf(
		"Show an outline of the document's headings beside it, as a comma separated list of "+
			"settings: style:%s, justify:%s. A page takes an 'outline' query parameter of the "+
			"same settings, which overrides this one",
		outlineStyleList(), strings.Join(outlineJustifications, "|")))
	list := flag.String("list", "", fmt.Sprintf(
		"List the documents around the one on the page, on its left, as a comma separated list "+
			"of settings: scope:%s. It takes the outline to the right, and a page takes a "+
			"'list' query parameter of the same settings",
		strings.Join(append(slices.Clone(listScopes), outlineNone), "|")))
	ado := flag.String("ado", "", fmt.Sprintf(
		"Read Azure Repos at the version named by a comma separated list of settings: %s. "+
			"One of them at a time, the repository's default branch without any; a page takes "+
			"an 'ado' query parameter of the same settings",
		strings.Join(adoVersionKinds, ":<name>, ")+":<name>"))
	contentWidth := flag.String("content-width", defaultContentWidth, fmt.Sprintf(
		"How wide the document is rendered: %s. The sidebars keep their own width, so a wider "+
			"document fills what they leave of the window",
		strings.Join(contentWidths, ", ")))
	search := flag.String("search", strings.Join(defaultSearch, ","),
		"The documents a folder is read as, a comma separated list looked through in the order "+
			"it is written, for local directories and Azure Repos folders alike")
	mermaid := flag.Bool("mermaid", false, "Render fenced 'mermaid' blocks as diagrams")
	indexOnly := flag.Bool("index-only", false,
		"Read a folder as the first document of --search it holds and look no further; a folder "+
			"holding none is served as one holding no Markdown at all, rather than as a listing "+
			"of the files it does hold")
	named := flag.String("config", "", fmt.Sprintf(
		"Read the flags from a YAML file, each written as the flag it is named after. Without "+
			"this, a %s beside the path is read when there is one; a flag on the command line "+
			"stands above what the file says",
		configName))
	showVersion := flag.Bool("version", false, "Print the version and exit")

	flag.Usage = printUsage
	flag.Parse()

	settings := configuration{path: flag.Arg(0), showVersion: *showVersion}
	if settings.showVersion {
		return settings, nil
	}

	// A flag written on the command line stands above what the file says, so the ones that
	// were written are noted before the file is read.
	written := map[string]bool{}
	flag.Visit(func(given *flag.Flag) { written[given.Name] = true })

	fromFile, file, err := readConfigFile(*named, settings.path)
	if err != nil {
		return configuration{}, err
	}
	settings.file = file

	settings.listenAddress = choose(written, "listen-address", *listenAddress, fromFile.ListenAddress)
	settings.port = choose(written, "port", *port, fromFile.Port)
	settings.watch = choose(written, "watch-interval", *watchInterval, fromFile.WatchInterval)
	settings.online = choose(written, "online", *online, fromFile.Online)
	settings.mermaid = choose(written, "mermaid", *mermaid, fromFile.Mermaid)
	settings.indexOnly = choose(written, "index-only", *indexOnly, fromFile.IndexOnly)

	settings.contentWidth = choose(written, "content-width", *contentWidth, fromFile.ContentWidth)
	if !slices.Contains(contentWidths, settings.contentWidth) {
		return configuration{}, fmt.Errorf("'%s' is not a content width; expected one of %s",
			settings.contentWidth, strings.Join(contentWidths, ", "))
	}

	settings.search = splitNames(*search)
	if !written["search"] && fromFile.Search != nil {
		settings.search = fromFile.Search.names
	}
	if len(settings.search) == 0 {
		return configuration{}, fmt.Errorf("'search' names no document; expected one or more, as %s",
			strings.Join(defaultSearch, ","))
	}

	// Without an outline the style is empty and the side still stands, for a query to turn
	// the outline on without naming one; a list is its scope alone, empty for no list.
	if settings.outline, err = chooseSettings(*outline, written["outline"], fromFile.Outline,
		outlineSettings{Justify: defaultOutline.Justify}, defaultOutline, parseOutline); err != nil {
		return configuration{}, err
	}
	if settings.list, err = chooseSettings(*list, written["list"], fromFile.List,
		listSettings{}, defaultList, parseList); err != nil {
		return configuration{}, err
	}
	if settings.ado, err = chooseSettings(*ado, written["ado"], fromFile.ADO,
		adoSettings{}, adoSettings{}, parseADO); err != nil {
		return configuration{}, err
	}
	return settings, nil
}

// printUsage writes what the server takes, the flags standing before the path.
func printUsage() {
	name := filepath.Base(os.Args[0])
	fmt.Fprintf(flag.CommandLine.Output(),
		"Usage: %s [flags] [path]\n\n"+
			"Render Markdown from local files or Azure Repos as GitHub-styled pages.\n\n"+
			"  path\n    \t%s\n\nFlags:\n",
		name, pathUsage)
	flag.PrintDefaults()
}

// choose returns what a setting is read as: the command line's value when the flag was
// written, the file's when it holds one, and the flag's own default otherwise.
func choose[T any](written map[string]bool, name string, onCommandLine T, fromFile *T) T {
	if fromFile != nil && !written[name] {
		return *fromFile
	}
	return onCommandLine
}

// chooseSettings returns the settings a flag taking a list is read with: the command line's
// when it was given, the file's when it holds them, and off when neither says anything.
//
// read is the flag's own reader, so a setting written in a file is refused for the same
// reasons it is on the command line.
func chooseSettings[T any](given string, onCommandLine bool, fromFile *settingsValue,
	off, defaults T, read func(string, T) (T, error)) (T, error) {

	if onCommandLine {
		if given == "" {
			return off, nil
		}
		return read(given, defaults)
	}
	if fromFile == nil || !fromFile.on {
		return off, nil
	}

	settings := defaults
	for _, key := range slices.Sorted(maps.Keys(fromFile.settings)) {
		asked, err := read(key+":"+fromFile.settings[key], settings)
		if err != nil {
			return off, err
		}
		settings = asked
	}
	return settings, nil
}

// fileConfig is what a configuration file holds: one key per flag, named as the flag is.
//
// A key left out keeps what the command line says, so a pointer tells a setting the file
// carries from one it says nothing about.
type fileConfig struct {
	ListenAddress *string        `yaml:"listen-address"`
	Port          *int           `yaml:"port"`
	WatchInterval *float64       `yaml:"watch-interval"`
	Outline       *settingsValue `yaml:"outline"`
	List          *settingsValue `yaml:"list"`
	ADO           *settingsValue `yaml:"ado"`
	Mermaid       *bool          `yaml:"mermaid"`
	Online        *bool          `yaml:"online"`
	IndexOnly     *bool          `yaml:"index-only"`
	ContentWidth  *string        `yaml:"content-width"`
	Search        *namesValue    `yaml:"search"`
}

// namesValue is how a list of names is written in a file: a sequence of them, or the comma
// separated list the command line takes.
type namesValue struct {
	names []string
}

// UnmarshalYAML reads a list of names from the node the file holds.
func (v *namesValue) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		var written string
		if err := node.Decode(&written); err != nil {
			return fmt.Errorf("line %d: expected a name, or a list of them", node.Line)
		}
		v.names = splitNames(written)
		return nil
	case yaml.SequenceNode:
		return node.Decode(&v.names)
	}
	return fmt.Errorf("line %d: expected a name, or a list of them", node.Line)
}

// splitNames reads a comma separated list of names, leaving out the empty ones and the space
// around each.
func splitNames(written string) []string {
	var names []string
	for _, name := range strings.Split(written, ",") {
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// settingsValue is how a flag taking a settings list is written in a file: true to draw it
// with its own settings, false to leave it out, or a mapping naming the settings themselves.
type settingsValue struct {
	on       bool
	settings map[string]string
}

// UnmarshalYAML reads a settings list from the node the file holds.
func (v *settingsValue) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if err := node.Decode(&v.on); err != nil {
			return fmt.Errorf("line %d: expected true, false or a mapping of settings", node.Line)
		}
		return nil
	case yaml.MappingNode:
		v.on = true
		return node.Decode(&v.settings)
	}
	return fmt.Errorf("line %d: expected true, false or a mapping of settings", node.Line)
}

// readConfigFile reads the configuration file the command line names, or the one beside the
// source when it names none.
//
// A file named by --config must be there; the one looked for beside the source need not be,
// and leaves the command line to say everything. The second result is the file that was read.
func readConfigFile(named, path string) (fileConfig, string, error) {
	var config fileConfig

	name, required := named, true
	if name == "" {
		name, required = filepath.Join(configDir(path), configName), false
	}

	content, err := os.ReadFile(name)
	if err != nil {
		if !required && errors.Is(err, fs.ErrNotExist) {
			return config, "", nil
		}
		return config, "", err
	}

	// Unknown keys are refused, so that a misspelled setting is told of rather than ignored.
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return fileConfig{}, "", fmt.Errorf("cannot read %s: %s", name, configError(err))
	}
	return config, name, nil
}

// configDir returns the directory the configuration file is looked for in: the one the path
// names, or the one the server was started in when it names a file or a repository.
func configDir(path string) string {
	switch {
	case path == "" || strings.HasPrefix(path, adoScheme):
		return "."
	case isDir(path):
		return path
	}
	return filepath.Dir(path)
}

// unknownKeyPattern matches what the YAML reader says of a key no flag is named after.
var unknownKeyPattern = regexp.MustCompile(`^(line \d+: )?field (\S+) not found in type \S+$`)

// configError returns what a file raised, with the reader's own words for a key no flag is
// named after put as the rest of the messages are.
func configError(err error) string {
	var typeError *yaml.TypeError
	if !errors.As(err, &typeError) {
		return err.Error()
	}

	messages := make([]string, 0, len(typeError.Errors))
	for _, message := range typeError.Errors {
		if named := unknownKeyPattern.FindStringSubmatch(message); named != nil {
			message = fmt.Sprintf("%s'%s' is not a setting", named[1], named[2])
		}
		messages = append(messages, message)
	}
	return strings.Join(messages, "; ")
}

// parseSettings reads a comma separated list of "key:value" pairs - "style:plain,
// justify:right" - passing each value to the reader its key names.
//
// It is how a settings list is read wherever it is written: a flag, a file, or a page's own
// query parameter. An item that is not a pair, a key no reader is named for, or a value its
// reader refuses, ends the reading and is returned as the error.
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

// outlineStyle returns the style given names, by its name, its shorthand or outlineNone for
// no outline. The second result is false when it names none of them.
func outlineStyle(given string) (string, bool) {
	if given == outlineNone {
		return "", true
	}
	for _, style := range outlineStyles {
		if given == style.name || given == style.shorthand {
			return style.name, true
		}
	}
	return "", false
}

// outlineStyleList names the styles with their shorthands, for the flags' help and their
// errors.
func outlineStyleList() string {
	names := make([]string, 0, len(outlineStyles)+1)
	for _, style := range outlineStyles {
		names = append(names, fmt.Sprintf("%s (%s)", style.name, style.shorthand))
	}
	return strings.Join(append(names, outlineNone), ", ")
}

// parseOutline reads the "style" and "justify" settings onto the ones it is given, and
// returns the outline the list asks for.
func parseOutline(given string, settings outlineSettings) (outlineSettings, error) {
	err := parseSettings(given, map[string]func(string) error{
		"style": func(value string) error {
			style, known := outlineStyle(value)
			if !known {
				return fmt.Errorf("'%s' is not an outline style; expected one of %s",
					value, outlineStyleList())
			}
			settings.Style = style
			return nil
		},
		"justify": settingFrom("an outline justification", outlineJustifications, &settings.Justify),
	})
	return settings, err
}

// parseList reads the "scope" setting onto the one it is given, and returns the list the
// setting asks for. A scope of outlineNone leaves no list at all.
func parseList(given string, settings listSettings) (listSettings, error) {
	scopes := append(slices.Clone(listScopes), outlineNone)
	err := parseSettings(given, map[string]func(string) error{
		"scope": func(value string) error {
			if err := settingFrom("a list scope", scopes, &settings.Scope)(value); err != nil {
				return err
			}
			if settings.Scope == outlineNone {
				settings.Scope = ""
			}
			return nil
		},
	})
	return settings, err
}
