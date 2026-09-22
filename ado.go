package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
)

// Azure DevOps AAD resource ID used to acquire access tokens for the REST API.
const adoResourceID = "499b84ac-1321-427f-aa17-267ca6975798"

// adoPATVariable is the environment variable a personal access token is read from.
const adoPATVariable = "AZURE_DEVOPS_PAT"

const adoGitAPIVersion = "7.1"

const adoTokenLifetime = 1800 * time.Second

// adoVersionKinds are the settings naming what a repository is read at, each the versionType
// the REST API knows it by.
var adoVersionKinds = []string{"branch", "tag", "commit"}

// adoSettings is what --ado and a page's 'ado' parameter say about reading Azure Repos: the
// version to read, empty for the repository's default branch, and which kind of version it
// is.
type adoSettings struct {
	Version     string
	VersionType string
}

// parseADO reads the "branch", "tag" and "commit" settings onto the ones it is given, and
// returns what the list asks for.
//
// One list names one version: a list naming two of them is refused, while a page's list
// replaces what the server was started with.
func parseADO(given string, settings adoSettings) (adoSettings, error) {
	named := ""
	reader := func(kind string) func(string) error {
		return func(value string) error {
			switch {
			case value == "":
				return fmt.Errorf("'%s' names no %s", given, kind)
			case named != "" && named != kind:
				return fmt.Errorf("'%s' names a version that '%s' has named already", kind, named)
			}
			named = kind
			settings.Version, settings.VersionType = value, kind
			return nil
		}
	}

	readers := map[string]func(string) error{}
	for _, kind := range adoVersionKinds {
		readers[kind] = reader(kind)
	}
	err := parseSettings(given, readers)
	return settings, err
}

// withVersion returns query carrying the version source is read at, which an item request
// needs and the listings of projects and repositories know nothing of.
func withVersion(src source, query url.Values) url.Values {
	if src.version != "" {
		query.Set("versionDescriptor.version", src.version)
		query.Set("versionDescriptor.versionType", src.versionType)
	}
	return query
}

// gitItem is the part of the Azure Repos GitItem answer this server reads.
type gitItem struct {
	Content  string `json:"content"`
	ObjectID string `json:"objectId"`
	IsFolder bool   `json:"isFolder"`
	Path     string `json:"path"`
}

// The access token is reused for adoTokenLifetime, since every poll of /content reads the
// file again.
var adoToken = struct {
	sync.Mutex
	value      string
	acquiredAt time.Time
}{}

// adoListings keeps what an organization, a project and a repository hold, since each is read
// to draw the pages below it, and each changes when someone creates or deletes a project, a
// repository, a branch or a tag rather than while a document is being read.
var adoListings = adoCache{held: map[string]adoHeld{}}

// adoCache keeps what was read from Azure DevOps under a name, each for adoTokenLifetime -
// the age the access token is kept for, so that there is one age to think about.
type adoCache struct {
	sync.Mutex
	held map[string]adoHeld
}

// adoHeld is one answer the cache holds, and when it was read.
type adoHeld struct {
	value  any
	readAt time.Time
}

// cached returns what name holds, reading it with read when nothing is held under it or what
// is held has aged out.
//
// The cache is held while the reading is done, so that pages asking for the same thing at
// once read it once; a read that fails is not kept, and is tried again by whoever asks next.
func cached[T any](cache *adoCache, name string, read func() (T, error)) (T, error) {
	cache.Lock()
	defer cache.Unlock()

	if held, found := cache.held[name]; found && time.Since(held.readAt) <= adoTokenLifetime {
		if value, held := held.value.(T); held {
			return value, nil
		}
	}

	value, err := read()
	if err != nil {
		var nothing T
		return nothing, err
	}
	cache.held[name] = adoHeld{value: value, readAt: time.Now()}
	return value, nil
}

// adoAuthorization returns the Authorization header an Azure DevOps request carries: the
// personal access token adoPATVariable holds, written as basic authentication under an empty
// user name, or a bearer token acquired through the az CLI.
func adoAuthorization() (string, error) {
	if pat := os.Getenv(adoPATVariable); pat != "" {
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(":"+pat)), nil
	}

	token, err := accessTokenFromAZ()
	if err != nil {
		return "", err
	}
	return "Bearer " + token, nil
}

// accessTokenFromAZ acquires an Azure DevOps access token through the az CLI, reusing the one
// last acquired until it is adoTokenLifetime old.
func accessTokenFromAZ() (string, error) {
	adoToken.Lock()
	defer adoToken.Unlock()

	if adoToken.value != "" && time.Since(adoToken.acquiredAt) <= adoTokenLifetime {
		return adoToken.value, nil
	}

	az, err := exec.LookPath("az")
	if err != nil {
		return "", fmt.Errorf("the az CLI is needed to read Azure Repos, but was not found on the "+
			"PATH; set %s to a personal access token to read them without it", adoPATVariable)
	}

	output, err := exec.Command(az, "account", "get-access-token",
		"--resource", adoResourceID, "--output", "json").Output()
	if err != nil {
		return "", fmt.Errorf("az account get-access-token failed: %w", err)
	}

	var answer struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.Unmarshal(output, &answer); err != nil {
		return "", fmt.Errorf("az account get-access-token did not answer with JSON")
	}
	if answer.AccessToken == "" {
		return "", fmt.Errorf("az account get-access-token returned no token")
	}

	adoToken.value = answer.AccessToken
	adoToken.acquiredAt = time.Now()
	return adoToken.value, nil
}

// parseADOLocation splits "<organization>[/<project>[/<repository>[<path to file>]]]" into an
// ado source.
//
// An organization and a project are folders of their own, holding the projects and the
// repositories below them; a location naming one addresses that listing. Within a repository,
// a location naming a folder, or ending in a slash, addresses the document inside it. The
// second result is false when location names not even an organization.
func parseADOLocation(location string) (source, bool) {
	var parts []string
	for _, part := range strings.Split(unescapePath(location), "/") {
		if part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return source{}, false
	}

	src := source{kind: kindADO, organization: parts[0]}
	if len(parts) > 1 {
		src.project = parts[1]
	}
	if len(parts) > 2 {
		src.repository = parts[2]
		src.path = "/" + strings.Join(parts[3:], "/")
		if len(parts) == 3 || strings.HasSuffix(location, "/") {
			src.path = strings.TrimSuffix(src.path, "/") + "/"
		}
	}
	return src, true
}

// parseADOURL splits an ado:// URL into the source it names, failing if it names nothing.
func parseADOURL(rawURL string) (source, error) {
	src, ok := parseADOLocation(strings.TrimPrefix(rawURL, adoScheme))
	if !ok {
		return source{}, fmt.Errorf("'%s' names no Azure DevOps organization; expected %s<organization>[/<project>[/<repository>[/<path to file>]]]",
			rawURL, adoScheme)
	}
	return src, nil
}

// adoWebURL returns the Azure DevOps web URL showing what source names: the organization, one
// of its projects, or a file in a repository.
func adoWebURL(src source) string {
	address := "https://dev.azure.com/" + url.PathEscape(src.organization)
	if src.project == "" {
		return address
	}
	address += "/" + url.PathEscape(src.project)
	if src.repository == "" {
		return address
	}

	// The path keeps its separators, the way Azure DevOps writes it in a browser's address bar.
	segments := strings.Split(src.path, "/")
	for index, segment := range segments {
		segments[index] = url.PathEscape(segment)
	}
	return fmt.Sprintf("%s/_git/%s?path=%s", address,
		url.PathEscape(src.repository), strings.Join(segments, "/"))
}

// readADOItem reads one path from the Azure Repos repository source names.
//
// itemPath defaults to the path source names when empty. $format=json is what the API
// documents for choosing JSON; the Accept header alone is not always honoured.
func readADOItem(src source, itemPath string, includeContent bool) (gitItem, error) {
	var item gitItem

	if itemPath == "" {
		itemPath = src.path
	}
	query := url.Values{}
	query.Set("$format", "json")
	query.Set("path", itemPath)
	query.Set("includeContent", fmt.Sprintf("%t", includeContent))
	query.Set("includeContentMetadata", "true")
	query.Set("api-version", adoGitAPIVersion)

	description := fmt.Sprintf("reading %s from %s", itemPath, src.repository)
	err := adoGetJSON(adoItemsURL(src, withVersion(src, query)), description, &item)
	return item, err
}

// readADORawItem reads the file source addresses as the bytes it holds, for a picture or
// another file that is not text.
//
// The item itself is read rather than JSON holding its content, and a Git LFS pointer is
// resolved to the file it stands for.
func readADORawItem(src source) ([]byte, error) {
	query := url.Values{}
	query.Set("path", src.path)
	query.Set("resolveLfs", "true")
	query.Set("api-version", adoGitAPIVersion)

	description := fmt.Sprintf("reading %s from %s", src.path, src.repository)
	return adoGet(adoItemsURL(src, withVersion(src, query)), "application/octet-stream", description)
}

// readADOItems lists the items below folder in the repository source names, one level down or
// the whole tree under it.
func readADOItems(src source, folder string, recursive bool) ([]gitItem, error) {
	recursion := "OneLevel"
	if recursive {
		recursion = "Full"
	}
	query := url.Values{}
	query.Set("$format", "json")
	query.Set("scopePath", folder)
	query.Set("recursionLevel", recursion)
	query.Set("api-version", adoGitAPIVersion)

	var answer struct {
		Value []gitItem `json:"value"`
	}
	description := fmt.Sprintf("listing %s in %s", folder, src.repository)
	err := adoGetJSON(adoItemsURL(src, withVersion(src, query)), description, &answer)
	return answer.Value, err
}

// readADOProjects lists the projects of the organization source names.
func readADOProjects(src source) ([]string, error) {
	return cached(&adoListings, "projects\n"+src.organization, func() ([]string, error) {
		query := url.Values{}
		query.Set("api-version", adoGitAPIVersion)

		var answer struct {
			Value []struct {
				Name string `json:"name"`
			} `json:"value"`
		}
		description := fmt.Sprintf("listing the projects of %s", src.organization)
		if err := adoGetJSON(adoURL(src, false, "_apis/projects", query), description, &answer); err != nil {
			return nil, err
		}

		names := make([]string, 0, len(answer.Value))
		for _, project := range answer.Value {
			names = append(names, project.Name)
		}
		return names, nil
	})
}

// adoRepository is a Git repository of a project, as the listing of them answers it.
type adoRepository struct {
	Name          string `json:"name"`
	DefaultBranch string `json:"defaultBranch"`
	IsDisabled    bool   `json:"isDisabled"`
}

// readADOProjectRepositories lists the Git repositories of the project source names, leaving
// out the ones that are disabled.
//
// The default branch of each comes with the listing, so a page asking which branch it reads
// costs no request of its own.
func readADOProjectRepositories(src source) ([]adoRepository, error) {
	name := "repositories\n" + src.organization + "\n" + src.project
	return cached(&adoListings, name, func() ([]adoRepository, error) {
		query := url.Values{}
		query.Set("api-version", adoGitAPIVersion)

		var answer struct {
			Value []adoRepository `json:"value"`
		}
		description := fmt.Sprintf("listing the repositories of %s", src.project)
		if err := adoGetJSON(adoURL(src, true, "_apis/git/repositories", query), description, &answer); err != nil {
			return nil, err
		}

		held := make([]adoRepository, 0, len(answer.Value))
		for _, repository := range answer.Value {
			if !repository.IsDisabled {
				held = append(held, repository)
			}
		}
		return held, nil
	})
}

// readADORepositories names the Git repositories of the project source names.
func readADORepositories(src source) ([]string, error) {
	repositories, err := readADOProjectRepositories(src)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(repositories))
	for _, repository := range repositories {
		names = append(names, repository.Name)
	}
	return names, nil
}

// readADODefaultBranch returns the branch the repository source names is read at when no
// version is asked for, empty when the repository says none.
func readADODefaultBranch(src source) (string, error) {
	repositories, err := readADOProjectRepositories(src)
	if err != nil {
		return "", err
	}

	for _, repository := range repositories {
		if strings.EqualFold(repository.Name, src.repository) {
			return refName(repository.DefaultBranch), nil
		}
	}
	return "", nil
}

// readADORefs names the branches or the tags of the repository source names, kind being the
// "heads" or "tags" the REST API files them under.
func readADORefs(src source, kind string) ([]string, error) {
	name := "refs " + kind + "\n" + src.organization + "\n" + src.project + "\n" + src.repository
	return cached(&adoListings, name, func() ([]string, error) {
		query := url.Values{}
		query.Set("filter", kind+"/")
		query.Set("$top", "1000")
		query.Set("api-version", adoGitAPIVersion)

		var answer struct {
			Value []struct {
				Name string `json:"name"`
			} `json:"value"`
		}
		description := fmt.Sprintf("listing the %s of %s", kind, src.repository)
		address := adoURL(src, true, "_apis/git/repositories/"+url.PathEscape(src.repository)+"/refs", query)
		if err := adoGetJSON(address, description, &answer); err != nil {
			return nil, err
		}

		names := make([]string, 0, len(answer.Value))
		for _, ref := range answer.Value {
			names = append(names, refName(ref.Name))
		}
		sort.SliceStable(names, func(i, j int) bool {
			return strings.ToLower(names[i]) < strings.ToLower(names[j])
		})
		return names, nil
	})
}

// refName returns the branch or tag a ref names, the API writing them as refs/heads/main and
// refs/tags/v1.0.
func refName(ref string) string {
	for _, prefix := range []string{"refs/heads/", "refs/tags/"} {
		if after, found := strings.CutPrefix(ref, prefix); found {
			return after
		}
	}
	return ref
}

// versionsOf returns what the page draws its branch and tag picker from, and nothing for a
// source that is not a document of an Azure Repos repository.
//
// The branches and the tags are both read, so that the page can offer either without asking
// again; a repository whose branches cannot be read carries no picker rather than a broken
// one.
func versionsOf(src source) *versionPicker {
	if src.kind != kindADO || src.repository == "" {
		return nil
	}

	branches, err := readADORefs(src, "heads")
	if err != nil {
		logInfo("  %scannot list the branches of %s: %v%s", colorYellow, src.repository, err, colorReset)
		return nil
	}
	tags, err := readADORefs(src, "tags")
	if err != nil {
		logInfo("  %scannot list the tags of %s: %v%s", colorYellow, src.repository, err, colorReset)
		tags = nil
	}
	defaultBranch, err := readADODefaultBranch(src)
	if err != nil {
		logInfo("  %scannot read the default branch of %s: %v%s", colorYellow, src.repository, err, colorReset)
	}

	return &versionPicker{DefaultBranch: defaultBranch, Branches: branches, Tags: tags,
		Kind: src.versionType, Version: src.version}
}

// adoRoute returns the route that addresses itemPath in what source names: the organization,
// one of its projects, or a repository holding the item.
func adoRoute(src source, itemPath string) string {
	route := "/" + routeNamespace + "/" + adoRouteName + "/" + url.PathEscape(src.organization)
	if src.project == "" {
		return route
	}
	route += "/" + url.PathEscape(src.project)
	if src.repository == "" {
		return route
	}
	return route + "/" + url.PathEscape(src.repository) + escapeRoute(itemPath)
}

// adoTree reads an Azure Repos repository as the tree the directory list is built from.
//
// folderPath is the folder the page stands in once it has been resolved, kept so that the
// repository is asked about it once. indexOnly leaves out the documents a folder is never
// read as. isFolder tells whether a repository path names a folder, and is the repository's
// own answer unless a test gives another.
type adoTree struct {
	src        source
	folderPath string
	search     indexSearch
	indexOnly  bool
	isFolder   func(source, string) bool
}

func (t adoTree) root() string { return "/" }

// folder returns the folder the addressed path lies in, which is the path itself when it
// names a folder rather than a document.
//
// A path ending in a slash, or carrying a Markdown suffix, says which it is on its own; any
// other is looked up, since a route reaching a folder is written without a trailing slash.
func (t adoTree) folder() string {
	if t.folderPath != "" {
		return t.folderPath
	}
	suffix := strings.ToLower(path.Ext(t.src.path))
	if strings.HasSuffix(t.src.path, "/") || markdownExtensions[suffix] {
		return path.Dir(t.src.path)
	}
	answer := t.isFolder
	if answer == nil {
		answer = adoItemIsFolder
	}
	if answer(t.src, t.src.path) {
		return t.src.path
	}
	return path.Dir(t.src.path)
}

func (t adoTree) document() string { return t.src.path }

func (t adoTree) parent(folder string) string { return path.Dir(folder) }

func (t adoTree) route(itemPath string) string { return adoRoute(t.src, itemPath) }

func (t adoTree) read(folder string, recursive bool) []treeItem {
	found, err := readADOItems(t.src, folder, recursive)
	if err != nil {
		logInfo("  %scannot list %s: %v%s", colorYellow, folder, err, colorReset)
		return nil
	}
	return t.items(found, folder)
}

// items keeps of what the repository answered with the folders and the documents that can be
// opened from folder, leaving out the hidden names and the files that are not Markdown.
//
// Under --index-only a folder holds the one document it is read as and no other, so a folder
// naming several of the documents --search names contributes the one that is read first.
func (t adoTree) items(found []gitItem, folder string) []treeItem {
	var items []treeItem
	documents := map[string]gitItem{}

	for _, found := range found {
		name := path.Base(found.Path)
		switch {
		case found.Path == "" || found.Path == folder || strings.HasPrefix(name, "."):
		case found.IsFolder:
			items = append(items, treeItem{path: found.Path, name: name, isFolder: true})
		case !markdownExtensions[strings.ToLower(path.Ext(found.Path))]:
		case !t.indexOnly:
			items = append(items, treeItem{path: found.Path, name: name})
		case t.search.holds(name):
			within := path.Dir(found.Path)
			held, taken := documents[within]
			if !taken || t.search.rank(name) < t.search.rank(path.Base(held.Path)) {
				documents[within] = found
			}
		}
	}

	for _, document := range documents {
		items = append(items, treeItem{path: document.Path, name: path.Base(document.Path)})
	}
	return items
}

// adoItemIsFolder reports whether itemPath names a folder in the repository source names.
//
// Reading a folder's content is an error rather than an empty answer, so this asks for its
// metadata alone, and only once a read has failed, to tell a folder apart from a path that is
// not there.
func adoItemIsFolder(src source, itemPath string) bool {
	item, err := readADOItem(src, itemPath, false)
	return err == nil && item.IsFolder
}

// errNoIndexDocument reports that a folder holds none of the documents a folder is read as.
var errNoIndexDocument = errors.New("no index document")

// readADODocument reads the document source addresses, resolving a folder to its index file.
//
// A folder is read the way a local directory is: the documents of the search are tried in
// order and the first one that exists is the document. A path that does not end in a slash is read
// directly, and looked up as a folder only if the repository says it is one.
//
// A folder holding none of them raises errNoIndexDocument, which the caller answers as the
// scope holding no Markdown at all.
func readADODocument(src source, search indexSearch) (itemPath, content, objectID string, err error) {
	itemPath = src.path
	if !strings.HasSuffix(itemPath, "/") {
		item, readErr := readADOItem(src, itemPath, true)
		if readErr == nil {
			return itemPath, item.Content, item.ObjectID, nil
		}
		var requestErr *adoRequestError
		if !errors.As(readErr, &requestErr) || !adoItemIsFolder(src, itemPath) {
			return "", "", "", readErr
		}
		itemPath += "/"
	}

	for _, candidate := range search {
		item, readErr := readADOItem(src, itemPath+candidate, true)
		if readErr != nil {
			continue
		}
		return itemPath + candidate, item.Content, item.ObjectID, nil
	}
	return "", "", "", fmt.Errorf("%w: none of %s found in '%s'",
		errNoIndexDocument, strings.Join(search, ", "), itemPath)
}

// readADOSource reads the document source addresses in Azure DevOps.
//
// An organization and a project hold no document of their own, so each is read as a listing:
// of the organization's projects, and of the project's repositories. A repository is read as
// the file the path names.
//
// With indexOnly, a folder holding neither of the documents a folder is read as is answered
// as one holding no Markdown at all, rather than with the error the read raised.
func readADOSource(src source, indexOnly bool, search indexSearch) (document, error) {
	switch {
	case src.project == "":
		names, err := readADOProjects(src)
		if err != nil {
			return document{}, err
		}
		return listingDocument(src.organization, "projects", names), nil

	case src.repository == "":
		names, err := readADORepositories(src)
		if err != nil {
			return document{}, err
		}
		return listingDocument(src.project, "repositories", names), nil
	}

	itemPath, content, objectID, err := readADODocument(src, search)
	if err != nil {
		if errors.Is(err, errNoIndexDocument) {
			return adoNothingRead(src, indexOnly, search), nil
		}
		return document{}, err
	}
	name := itemPath
	if index := strings.LastIndex(name, "/"); index >= 0 {
		name = name[index+1:]
	}
	css, markdown := asMarkdownDocument(name, content)
	return document{marker: objectID, name: name, text: markdown, css: css}, nil
}

// adoNothingRead returns the document a folder holding neither of the documents a folder is
// read as is served as: one titled after the folder, or after the repository at its root, so
// that the page names what was read rather than a message alone.
//
// Under --index-only nothing else was looked for; otherwise the documents the folder does
// hold are named by the list beside the page.
func adoNothingRead(src source, indexOnly bool, search indexSearch) document {
	folder := strings.TrimSuffix(src.path, "/")
	if folder == "" || folder == "/" {
		folder = src.repository
	}

	title := path.Base(folder)
	if indexOnly {
		return listingDocument(title, "Markdown files", nil)
	}
	return textDocument(fmt.Sprintf("# %s\n\nNo %s found here.\n", title, search.named()))
}

// listingDocument returns the document listing what an organization or a project holds, each
// name linking to the route below the one the page stands at.
//
// held names what is listed, for the line a listing without any of them carries.
func listingDocument(title, held string, names []string) document {
	sort.SliceStable(names, func(i, j int) bool {
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})

	lines := []string{"# " + title, ""}
	if len(names) == 0 {
		lines = append(lines, fmt.Sprintf("No %s found.", held))
	}
	for _, name := range names {
		lines = append(lines, fmt.Sprintf("- [%s](%s)", name, url.PathEscape(name)))
	}

	return textDocument(strings.Join(lines, "\n") + "\n")
}

// textDocument returns a document the server wrote itself, marked by the text it holds, since
// it has no modification time or object ID of its own.
func textDocument(text string) document {
	return document{marker: textMarker(text), text: text}
}

// adoList returns the directory list beside a page of an Azure DevOps source.
//
// An organization and a project carry the organization's projects, the one on the page
// marked; a repository carries its own documents, under the link back to the repositories of
// its project. The scope says how far below the entries the list reaches, the way it does for
// a local directory.
//
// A repository root whose list leads nowhere - holding no document at all, or the one already
// on the page and nothing else, as it does under --index-only once the documents that are
// never read are left out - carries the project's repositories instead, the one on the page
// marked. The link above them leads where it always does, under the label that names
// what the list now holds. A folder below the root keeps its own list, however short, since
// the folder above it is where the way out lies.
func adoList(src source, scope string, indexOnly bool, search indexSearch) ([]listEntry, listLink) {
	if src.repository == "" {
		return adoProjectEntries(src, scope), listLink{}
	}

	tree := adoTree{src: src, indexOnly: indexOnly, search: search}
	tree.folderPath = tree.folder()

	entries := documentTree(tree, scope)
	up := adoListLink(src, tree.folderPath)
	if tree.folderPath == tree.root() && holdsNothingToBrowse(entries, search) {
		entries = adoRepositoryEntries(src, src.project)
		up.Label = "Back to projects"
	}
	return entries, up
}

// adoListLink returns the link standing above a repository's documents: the folder above the
// one the page stands in, or the repositories of the project once the page stands at the
// repository's root.
func adoListLink(src source, folder string) listLink {
	if folder != "" && folder != "/" {
		return listLink{Label: "Up", Route: adoRoute(src, path.Dir(folder))}
	}
	return adoUpLink(src)
}

// adoProjectEntries lists the projects of the organization source names, the one on the page
// marked, with the repositories of the projects the scope reaches nested below them.
//
// The scope reaches the project on the page below "subfolders", and every project below
// "tree", which reads the repositories of each in turn.
func adoProjectEntries(src source, scope string) []listEntry {
	names, err := readADOProjects(src)
	if err != nil {
		logInfo("  %scannot list the projects of %s: %v%s", colorYellow, src.organization, err, colorReset)
		return nil
	}

	entries := namedEntries(names, src.project, func(name string) string {
		return adoRoute(source{kind: kindADO, organization: src.organization, project: name}, "")
	})
	for index, entry := range entries {
		if scope == "tree" || (scope == "subfolders" && entry.Current) {
			entries[index].Children = adoRepositoryEntries(src, entry.Name)
		}
	}
	return entries
}

// adoRepositoryEntries lists the Git repositories of one project of the organization source
// names, the repository on the page marked.
func adoRepositoryEntries(src source, project string) []listEntry {
	within := source{kind: kindADO, organization: src.organization, project: project}
	names, err := readADORepositories(within)
	if err != nil {
		logInfo("  %scannot list the repositories of %s: %v%s", colorYellow, project, err, colorReset)
		return nil
	}
	return namedEntries(names, src.repository, func(name string) string {
		return adoRoute(source{kind: kindADO, organization: src.organization,
			project: project, repository: name}, "")
	})
}

// namedEntries turns names into list entries addressing the route each name names, marking
// the one the page stands at.
func namedEntries(names []string, current string, route func(string) string) []listEntry {
	entries := make([]listEntry, 0, len(names))
	for _, name := range names {
		entries = append(entries, listEntry{Name: name, Route: route(name), Current: name == current})
	}
	sortEntries(entries)
	return entries
}

// holdsNothingToBrowse reports whether entries lead nowhere: they are empty, or hold the
// document the page shows and nothing else.
//
// A route naming a folder - a repository's own route among them - addresses the document
// within it without naming it, so the entry cannot be marked; an only entry named as one of
// the indexNames is that document, whatever its letters' case, since the folder resolves to
// it.
func holdsNothingToBrowse(entries []listEntry, search indexSearch) bool {
	if len(entries) == 0 {
		return true
	}
	if len(entries) != 1 || len(entries[0].Children) != 0 {
		return false
	}
	return entries[0].Current || search.holds(entries[0].Name)
}

// adoUpLink returns the link standing above a repository's documents, leading back to the
// repositories of the project it belongs to.
func adoUpLink(src source) listLink {
	if src.repository == "" {
		return listLink{}
	}
	project := source{kind: kindADO, organization: src.organization, project: src.project}
	return listLink{Label: "Browse to repositories", Route: adoRoute(project, "")}
}
