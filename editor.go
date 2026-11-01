package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/skoszewski/serve-markdown/v3/render"
	"go.yaml.in/yaml/v3"
)

// editorRoute is the URL prefix the Markdown editor and its API are served under.
const editorRoute = "/" + routeNamespace + "/editor/"

// newFilePattern is the path a new document is created at: folders and a name of word
// characters, dots and dashes, ending in a Markdown suffix.
var newFilePattern = regexp.MustCompile(`^([\w.-]+/)*[\w.-]+\.(md|markdown)$`)

// A new document is titled after its name, or after its folder for an index or README
// document, the dashes and underscores read as spaces.
var (
	indexNamePattern      = regexp.MustCompile(`(?i)/(index|readme)$`)
	titleSeparatorPattern = regexp.MustCompile(`[-_]+`)
)

// editorConfigName is the file in the served directory naming the folders the editor edits and
// the paths it leaves out.
const editorConfigName = "editor.json"

// editor serves the Markdown editor of the documents below rootDir that are not excluded,
// saved in place, and created empty or from the templates in templatesDir.
//
// directories are the folders below rootDir whose documents are edited, all of rootDir when
// empty; ignore matches the paths left out of editing, none when nil; spellCheck shows the
// spell check controls in the editor's top bar.
type editor struct {
	rootDir      string
	templatesDir string
	directories  []string
	ignore       *ignoreRules
	spellCheck   bool
}

// editorConfig is what editorConfigName holds: the folders relative to the served directory
// whose documents are edited, and the .gitignore patterns of the paths left out of editing.
type editorConfig struct {
	Directories []string `json:"directories"`
	Exclude     []string `json:"exclude"`
}

// readEditorConfig reads the editor configuration file, failing with an error that wraps
// fs.ErrNotExist when there is no such file. A folder must be relative and stay below the
// served directory, and a pattern must read as a .gitignore pattern.
func readEditorConfig(file string) (editorConfig, error) {
	var config editorConfig
	content, err := os.ReadFile(file)
	if err != nil {
		return config, err
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return config, fmt.Errorf("cannot read %s: %v", file, err)
	}
	for index, directory := range config.Directories {
		directory = strings.Trim(path.Clean(filepath.ToSlash(directory)), "/")
		if filepath.IsAbs(config.Directories[index]) || strings.HasPrefix(config.Directories[index], "/") ||
			directory == ".." || strings.HasPrefix(directory, "../") {
			return config, fmt.Errorf("%s: '%s' is not a folder below the served directory", file, config.Directories[index])
		}
		config.Directories[index] = directory
	}
	if _, err := readIgnoreRules("", config.Exclude); err != nil {
		return config, fmt.Errorf("%s: %v", file, err)
	}
	return config, nil
}

// inDirectories reports whether a path relative to the served directory lies in one of the
// folders the editor edits.
func (e *editor) inDirectories(relative string) bool {
	if len(e.directories) == 0 {
		return true
	}
	return slices.ContainsFunc(e.directories, func(directory string) bool {
		return directory == "." || strings.HasPrefix(relative, directory+"/")
	})
}

// editorProperties are the properties of a document the editor keeps in its front matter.
type editorProperties struct {
	Title   string   `json:"title,omitempty"`
	Author  string   `json:"author,omitempty"`
	Created string   `json:"created,omitempty"`
	Updated string   `json:"updated,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

// editorTemplate describes a template from its front matter.
type editorTemplate struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Author      string `json:"author"`
	Description string `json:"description"`
}

// gitState is the Git state the editor shows: the current branch, the branches, the changed
// editable files and the numbers of commits not pushed and not pulled, null without an
// upstream branch.
type gitState struct {
	Branch   string   `json:"branch"`
	Branches []string `json:"branches"`
	Changes  []string `json:"changes"`
	Ahead    *int     `json:"ahead"`
	Behind   *int     `json:"behind"`
}

// ServeHTTP serves the editor application at editorRoute and its API below it.
func (e *editor) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	name := strings.TrimPrefix(request.URL.Path, editorRoute)
	if name == "" {
		shell, err := pageFS.ReadFile("assets/page/editor.html")
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		writer.Write(shell)
		return
	}
	if !strings.HasPrefix(name, "api/") {
		http.NotFound(writer, request)
		return
	}

	// A request changing a file comes from the editor's own page: another site's page sends
	// its own origin, and a form cannot send JSON.
	if request.Method != http.MethodGet {
		if origin := request.Header.Get("Origin"); origin != "" {
			if parsed, err := url.Parse(origin); err != nil || parsed.Host != request.Host {
				sendText(writer, http.StatusForbidden, "The request comes from another origin.")
				return
			}
		}
		if mediaType, _, _ := mime.ParseMediaType(request.Header.Get("Content-Type")); request.ContentLength != 0 &&
			mediaType != "application/json" {
			sendText(writer, http.StatusUnsupportedMediaType, "The request body must be JSON.")
			return
		}
	}

	if err := e.serveAPI(writer, request, strings.TrimPrefix(name, "api/")); err != nil {
		sendText(writer, http.StatusInternalServerError, err.Error())
	}
}

// serveAPI answers the editor's API:
//   - GET config returns whether the spell check controls are shown;
//   - GET files lists the editable files;
//   - GET templates lists the templates;
//   - GET file?path=<path> returns the properties and the body of a document;
//   - PUT file?path=<path> saves the title, author, tags and body of a document and sets its
//     update time;
//   - POST file?path=<path> creates a file, from the template the request body names;
//   - git and git/<action>, see serveGit.
func (e *editor) serveAPI(writer http.ResponseWriter, request *http.Request, name string) error {
	if name == "git" || strings.HasPrefix(name, "git/") {
		if err := e.serveGit(writer, request, strings.TrimPrefix(strings.TrimPrefix(name, "git"), "/")); err != nil {
			sendText(writer, http.StatusConflict, err.Error())
		}
		return nil
	}

	switch {
	case name == "config" && request.Method == http.MethodGet:
		sendJSON(writer, http.StatusOK, map[string]bool{"spellCheckControls": e.spellCheck})
		return nil
	case name == "files" && request.Method == http.MethodGet:
		files, err := e.listFiles()
		if err != nil {
			return err
		}
		sendJSON(writer, http.StatusOK, files)
		return nil
	case name == "templates" && request.Method == http.MethodGet:
		templates, err := e.listTemplates()
		if err != nil {
			return err
		}
		sendJSON(writer, http.StatusOK, templates)
		return nil
	case name != "file":
		sendText(writer, http.StatusNotFound, "Not Found")
		return nil
	}

	relative := request.URL.Query().Get("path")
	if request.Method == http.MethodPost {
		var body struct {
			Template string `json:"template"`
		}
		if err := readJSON(request, &body); err != nil {
			sendText(writer, http.StatusBadRequest, err.Error())
			return nil
		}
		return e.createFile(writer, relative, body.Template)
	}

	files, err := e.listFiles()
	if err != nil {
		return err
	}
	if !slices.Contains(files, relative) {
		sendText(writer, http.StatusNotFound, "Unknown file: "+relative)
		return nil
	}
	file := filepath.Join(e.rootDir, filepath.FromSlash(relative))

	switch request.Method {
	case http.MethodGet:
		frontMatter, body, err := readEditorDocument(file)
		if err != nil {
			return err
		}
		sendJSON(writer, http.StatusOK, map[string]any{"properties": readProperties(frontMatter), "body": body})
	case http.MethodPut:
		var saved struct {
			Properties editorProperties `json:"properties"`
			Body       string           `json:"body"`
		}
		if err := readJSON(request, &saved); err != nil {
			sendText(writer, http.StatusBadRequest, err.Error())
			return nil
		}
		frontMatter, _, err := readEditorDocument(file)
		if err != nil {
			return err
		}
		setScalar(frontMatter, "title", saved.Properties.Title)
		setScalar(frontMatter, "author", saved.Properties.Author)
		setTags(frontMatter, saved.Properties.Tags)
		setTimestamp(frontMatter, "updated", time.Now())
		if err := writeEditorDocument(file, frontMatter, saved.Body, false); err != nil {
			return err
		}
		sendJSON(writer, http.StatusOK, readProperties(frontMatter))
	default:
		sendText(writer, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
	return nil
}

// serveGit answers the editor's Git actions, run in the served directory:
//   - GET with no action returns the gitState;
//   - POST fetch fetches all remotes, removing the deleted remote branches;
//   - POST pull fast-forwards the current branch to its upstream branch;
//   - POST commit adds and commits the changed editable files with the message the request
//     body holds;
//   - POST push pushes the current branch, setting its upstream branch on the first remote
//     when it has none;
//   - POST switch switches to the branch the request body names.
//
// A failed Git command is returned as the error.
func (e *editor) serveGit(writer http.ResponseWriter, request *http.Request, action string) error {
	switch {
	case action == "" && request.Method == http.MethodGet:
		state := gitState{}
		branch, err := e.git("branch", "--show-current")
		if err != nil {
			return err
		}
		state.Branch = strings.TrimSpace(branch)
		if state.Branches, err = e.listBranches(); err != nil {
			return err
		}
		if state.Changes, err = e.changedFiles(); err != nil {
			return err
		}
		state.Ahead, state.Behind = e.commitCount("@{upstream}..HEAD"), e.commitCount("HEAD..@{upstream}")
		sendJSON(writer, http.StatusOK, state)
	case action == "fetch" && request.Method == http.MethodPost:
		if _, err := e.git("fetch", "--all", "--prune"); err != nil {
			return err
		}
		writer.WriteHeader(http.StatusNoContent)
	case action == "pull" && request.Method == http.MethodPost:
		if _, err := e.git("pull", "--ff-only"); err != nil {
			return err
		}
		writer.WriteHeader(http.StatusNoContent)
	case action == "commit" && request.Method == http.MethodPost:
		var body struct {
			Message string `json:"message"`
		}
		if err := readJSON(request, &body); err != nil {
			sendText(writer, http.StatusBadRequest, err.Error())
			return nil
		}
		files, err := e.changedFiles()
		if err != nil {
			return err
		}
		message := strings.TrimSpace(body.Message)
		switch {
		case message == "":
			sendText(writer, http.StatusBadRequest, "The commit message is empty.")
		case len(files) == 0:
			sendText(writer, http.StatusConflict, "There are no changes to commit.")
		default:
			if _, err := e.git(append([]string{"add", "--"}, files...)...); err != nil {
				return err
			}
			if _, err := e.git(append([]string{"commit", "--message", message, "--"}, files...)...); err != nil {
				return err
			}
			sendJSON(writer, http.StatusOK, files)
		}
	case action == "push" && request.Method == http.MethodPost:
		if _, err := e.git("rev-parse", "--abbrev-ref", "@{upstream}"); err == nil {
			if _, err := e.git("push"); err != nil {
				return err
			}
		} else {
			remotes, err := e.git("remote")
			if err != nil {
				return err
			}
			remote, _, _ := strings.Cut(strings.TrimSpace(remotes), "\n")
			if remote == "" {
				sendText(writer, http.StatusConflict, "The repository has no remote.")
				return nil
			}
			branch, err := e.git("branch", "--show-current")
			if err != nil {
				return err
			}
			if _, err := e.git("push", "--set-upstream", remote, strings.TrimSpace(branch)); err != nil {
				return err
			}
		}
		writer.WriteHeader(http.StatusNoContent)
	case action == "switch" && request.Method == http.MethodPost:
		var body struct {
			Branch string `json:"branch"`
		}
		if err := readJSON(request, &body); err != nil {
			sendText(writer, http.StatusBadRequest, err.Error())
			return nil
		}
		branches, err := e.listBranches()
		if err != nil {
			return err
		}
		if body.Branch == "" || !slices.Contains(branches, body.Branch) {
			sendText(writer, http.StatusBadRequest, "Unknown branch: "+body.Branch)
			return nil
		}
		if _, err := e.git("switch", body.Branch); err != nil {
			return err
		}
		writer.WriteHeader(http.StatusNoContent)
	default:
		sendText(writer, http.StatusNotFound, "Not Found")
	}
	return nil
}

// listFiles returns the documents below the served directory that are not excluded, as sorted paths relative to it with forward slashes. Names beginning with a dot
// are left out, as the directory list leaves them.
func (e *editor) listFiles() ([]string, error) {
	var files []string
	err := filepath.WalkDir(e.rootDir, func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if file == e.rootDir {
			return nil
		}
		relative, err := filepath.Rel(e.rootDir, file)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if strings.HasPrefix(entry.Name(), ".") || e.ignore.ignored(relative, entry.IsDir()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type().IsRegular() && render.MarkdownExtensions[strings.ToLower(filepath.Ext(entry.Name()))] &&
			e.inDirectories(relative) {
			files = append(files, relative)
		}
		return nil
	})
	slices.Sort(files)
	return files, err
}

// listTemplates returns the templates of the templates directory sorted by title, none
// without the directory.
func (e *editor) listTemplates() ([]editorTemplate, error) {
	templates := []editorTemplate{}
	if e.templatesDir == "" {
		return templates, nil
	}
	entries, err := os.ReadDir(e.templatesDir)
	if errors.Is(err, fs.ErrNotExist) {
		return templates, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !render.MarkdownExtensions[strings.ToLower(filepath.Ext(entry.Name()))] {
			continue
		}
		frontMatter, _, err := readEditorDocument(filepath.Join(e.templatesDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		template := editorTemplate{Name: entry.Name(), Title: scalarValue(frontMatter, "title"),
			Author: scalarValue(frontMatter, "author"), Description: scalarValue(frontMatter, "description")}
		if template.Title == "" {
			template.Title = entry.Name()
		}
		templates = append(templates, template)
	}
	slices.SortFunc(templates, func(a, b editorTemplate) int { return strings.Compare(a.Title, b.Title) })
	return templates, nil
}

// createFile creates a document below the served directory, with its folders. Its front
// matter gets the title from the file name, or the folder
// name for an index or README document, and the creation time. A document from a template
// gets the body of the template and the author "user.name (user.email)" from the Git
// configuration, or the one of them that is defined.
func (e *editor) createFile(writer http.ResponseWriter, relative, templateName string) error {
	if !newFilePattern.MatchString(relative) || slices.ContainsFunc(strings.Split(relative, "/"),
		func(segment string) bool { return strings.HasPrefix(segment, ".") }) {
		sendText(writer, http.StatusBadRequest, "The path must be a .md or .markdown file "+
			"below the served directory, without names beginning with a dot: "+relative)
		return nil
	}
	if e.ignore.ignored(relative, false) {
		sendText(writer, http.StatusBadRequest, "The path is excluded from editing: "+relative)
		return nil
	}
	if !e.inDirectories(relative) {
		sendText(writer, http.StatusBadRequest, fmt.Sprintf("The path must be in %s: %s",
			strings.Join(e.directories, ", "), relative))
		return nil
	}
	file := filepath.Join(e.rootDir, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(file), 0o777); err != nil {
		return err
	}

	body, author := "", ""
	if templateName != "" {
		templates, err := e.listTemplates()
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(templates, func(template editorTemplate) bool { return template.Name == templateName }) {
			sendText(writer, http.StatusBadRequest, "Unknown template: "+templateName)
			return nil
		}
		if _, body, err = readEditorDocument(filepath.Join(e.templatesDir, templateName)); err != nil {
			return err
		}
		name, _ := e.git("config", "user.name")
		email, _ := e.git("config", "user.email")
		name, email = strings.TrimSpace(name), strings.TrimSpace(email)
		author = name + email
		if name != "" && email != "" {
			author = fmt.Sprintf("%s (%s)", name, email)
		}
	}

	title := strings.TrimSuffix(strings.TrimSuffix(relative, ".md"), ".markdown")
	title = titleSeparatorPattern.ReplaceAllString(path.Base(indexNamePattern.ReplaceAllString(title, "")), " ")

	frontMatter := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	now := time.Now()
	setScalar(frontMatter, "title", title)
	setScalar(frontMatter, "author", author)
	setTimestamp(frontMatter, "created", now)
	setTimestamp(frontMatter, "updated", now)
	err := writeEditorDocument(file, frontMatter, body, true)
	if errors.Is(err, fs.ErrExist) {
		sendText(writer, http.StatusConflict, "The file exists: "+relative)
		return nil
	}
	if err != nil {
		return err
	}
	writer.WriteHeader(http.StatusCreated)
	return nil
}

// git runs a Git command in the served directory without terminal prompts and returns its
// standard output, or its standard error as the error of a failed command.
func (e *editor) git(args ...string) (string, error) {
	command := exec.Command("git", args...)
	command.Dir = e.rootDir
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return "", errors.New(message)
		}
		return "", err
	}
	return stdout.String(), nil
}

// commitCount returns the number of commits in a Git revision range, nil when it cannot be
// counted, as for a branch without an upstream branch.
func (e *editor) commitCount(revisions string) *int {
	output, err := e.git("rev-list", "--count", revisions)
	if err != nil {
		return nil
	}
	count, err := strconv.Atoi(strings.TrimSpace(output))
	if err != nil {
		return nil
	}
	return &count
}

// changedFiles returns the editable files with uncommitted changes, new files included, as
// sorted paths relative to the served directory. Changes of other files are left out.
func (e *editor) changedFiles() ([]string, error) {
	prefix, err := e.git("rev-parse", "--show-prefix")
	if err != nil {
		return nil, err
	}
	prefix = strings.TrimSpace(prefix)
	status, err := e.git("status", "--porcelain=v1", "-z", "--untracked-files=all", "--", ".")
	if err != nil {
		return nil, err
	}
	files, err := e.listFiles()
	if err != nil {
		return nil, err
	}

	// A renamed or copied file is followed by the path it was renamed or copied from.
	changes := []string{}
	entries := strings.Split(status, "\x00")
	for index := 0; index < len(entries); index++ {
		entry := entries[index]
		if len(entry) <= 3 {
			continue
		}
		if entry[0] == 'R' || entry[0] == 'C' {
			index++
		}
		if relative, found := strings.CutPrefix(entry[3:], prefix); found && slices.Contains(files, relative) {
			changes = append(changes, relative)
		}
	}
	slices.Sort(changes)
	return changes, nil
}

// listBranches returns the local branches and the branches of the remotes, without the
// remote names, sorted.
func (e *editor) listBranches() ([]string, error) {
	output, err := e.git("remote")
	if err != nil {
		return nil, err
	}
	remotes := strings.Fields(output)
	output, err = e.git("for-each-ref", "--format=%(refname:short)", "refs/heads", "refs/remotes")
	if err != nil {
		return nil, err
	}
	branches := []string{}
	for _, ref := range strings.Fields(output) {
		if slices.Contains(remotes, ref) || strings.HasSuffix(ref, "/HEAD") {
			continue
		}
		for _, remote := range remotes {
			if trimmed, found := strings.CutPrefix(ref, remote+"/"); found {
				ref = trimmed
				break
			}
		}
		if !slices.Contains(branches, ref) {
			branches = append(branches, ref)
		}
	}
	slices.Sort(branches)
	return branches, nil
}

// readEditorDocument reads a document and returns its front matter as a YAML mapping, empty
// without one, and its body without the leading blank lines.
func readEditorDocument(file string) (*yaml.Node, string, error) {
	content, err := os.ReadFile(file)
	if err != nil {
		return nil, "", err
	}
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	mapping := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	match := render.FrontMatterPattern.FindStringSubmatchIndex(text)
	if match == nil {
		return mapping, strings.TrimLeft(text, "\n"), nil
	}
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(text[match[2]:match[3]]), &document); err != nil {
		return nil, "", fmt.Errorf("invalid front matter in %s: %v", file, err)
	}
	if len(document.Content) > 0 {
		if document.Content[0].Kind != yaml.MappingNode {
			return nil, "", fmt.Errorf("the front matter of %s is not a mapping", file)
		}
		mapping = document.Content[0]
	}
	return mapping, strings.TrimLeft(text[match[1]:], "\n"), nil
}

// writeEditorDocument writes a document with its front matter, left out when empty, and its
// body ending in a line break unless it is empty. A new file fails with fs.ErrExist when the file exists.
func writeEditorDocument(file string, frontMatter *yaml.Node, body string, newFile bool) error {
	var content bytes.Buffer
	if len(frontMatter.Content) > 0 {
		content.WriteString("---\n")
		encoder := yaml.NewEncoder(&content)
		encoder.SetIndent(2)
		if err := encoder.Encode(frontMatter); err != nil {
			return err
		}
		if err := encoder.Close(); err != nil {
			return err
		}
		content.WriteString("---\n\n")
	}
	text := strings.TrimLeft(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	content.WriteString(text)
	if text != "" && !strings.HasSuffix(text, "\n") {
		content.WriteString("\n")
	}
	if newFile {
		return writeNewFile(file, content.Bytes())
	}
	return os.WriteFile(file, content.Bytes(), 0o666)
}

// writeNewFile writes a file that must not exist yet, failing with fs.ErrExist when it does.
func writeNewFile(file string, content []byte) error {
	created, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		return err
	}
	if _, err := created.Write(content); err != nil {
		created.Close()
		return err
	}
	return created.Close()
}

// readProperties returns the properties a front matter mapping holds.
func readProperties(frontMatter *yaml.Node) editorProperties {
	properties := editorProperties{Title: scalarValue(frontMatter, "title"), Author: scalarValue(frontMatter, "author"),
		Created: scalarValue(frontMatter, "created"), Updated: scalarValue(frontMatter, "updated")}
	if tags := mappingValue(frontMatter, "tags"); tags != nil && tags.Kind == yaml.SequenceNode {
		for _, tag := range tags.Content {
			if tag.Kind == yaml.ScalarNode {
				properties.Tags = append(properties.Tags, tag.Value)
			}
		}
	}
	return properties
}

// mappingValue returns the value of a key in a YAML mapping, nil without the key.
func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index+1]
		}
	}
	return nil
}

// scalarValue returns the text of a single value in a YAML mapping, empty for another kind of
// value or without the key.
func scalarValue(mapping *yaml.Node, key string) string {
	if value := mappingValue(mapping, key); value != nil && value.Kind == yaml.ScalarNode {
		return value.Value
	}
	return ""
}

// setValue sets a key of a YAML mapping to the value its node encodes, keeping the comments of
// a replaced value, and removes the key when value is nil.
func setValue(mapping *yaml.Node, key string, value any) {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value != key {
			continue
		}
		if value == nil {
			mapping.Content = slices.Delete(mapping.Content, index, index+2)
			return
		}
		replaced := mapping.Content[index+1]
		node := &yaml.Node{}
		node.Encode(value)
		node.HeadComment, node.LineComment, node.FootComment = replaced.HeadComment, replaced.LineComment, replaced.FootComment
		mapping.Content[index+1] = node
		return
	}
	if value != nil {
		node := &yaml.Node{}
		node.Encode(value)
		mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, node)
	}
}

// setScalar sets a text value of a front matter mapping, removing it when empty.
func setScalar(mapping *yaml.Node, key, value string) {
	if value = strings.TrimSpace(value); value == "" {
		setValue(mapping, key, nil)
		return
	}
	setValue(mapping, key, value)
}

// setTags sets the tags of a front matter mapping, removing them when there are none.
func setTags(mapping *yaml.Node, tags []string) {
	var kept []string
	for _, tag := range tags {
		if tag = strings.TrimSpace(tag); tag != "" {
			kept = append(kept, tag)
		}
	}
	if len(kept) == 0 {
		setValue(mapping, "tags", nil)
		return
	}
	setValue(mapping, "tags", kept)
}

// setTimestamp sets a time of a front matter mapping as a UTC YAML timestamp in seconds.
func setTimestamp(mapping *yaml.Node, key string, moment time.Time) {
	setValue(mapping, key, moment.UTC().Truncate(time.Second))
}

// readJSON decodes the JSON body of a request into target.
func readJSON(request *http.Request, target any) error {
	if err := json.NewDecoder(request.Body).Decode(target); err != nil {
		return fmt.Errorf("the request body is not valid JSON: %v", err)
	}
	return nil
}

// sendJSON answers with a value as JSON.
func sendJSON(writer http.ResponseWriter, status int, value any) {
	content, err := json.Marshal(value)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	writer.Write(content)
}

// sendText answers with a plain text message.
func sendText(writer http.ResponseWriter, status int, message string) {
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writer.WriteHeader(status)
	writer.Write([]byte(message))
}
