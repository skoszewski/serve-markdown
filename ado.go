package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
)

// Azure DevOps AAD resource ID used to acquire access tokens for the REST API.
const adoResourceID = "499b84ac-1321-427f-aa17-267ca6975798"

const adoGitAPIVersion = "7.1"

const adoTokenLifetime = 1800 * time.Second

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
		return "", fmt.Errorf("the az CLI is needed to read Azure Repos, but was not found on the PATH")
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
	err := adoGetJSON(adoURL(src, true, "_apis/git/repositories/"+url.PathEscape(src.repository)+"/items", query),
		description, &item)
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
	return adoGet(adoURL(src, true, "_apis/git/repositories/"+url.PathEscape(src.repository)+"/items", query),
		"application/octet-stream", description)
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
	err := adoGetJSON(adoURL(src, true, "_apis/git/repositories/"+url.PathEscape(src.repository)+"/items", query),
		description, &answer)
	return answer.Value, err
}

// readADOProjects lists the projects of the organization source names.
func readADOProjects(src source) ([]string, error) {
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
}

// readADORepositories lists the Git repositories of the project source names, leaving out the
// ones that are disabled.
func readADORepositories(src source) ([]string, error) {
	query := url.Values{}
	query.Set("api-version", adoGitAPIVersion)

	var answer struct {
		Value []struct {
			Name       string `json:"name"`
			IsDisabled bool   `json:"isDisabled"`
		} `json:"value"`
	}
	description := fmt.Sprintf("listing the repositories of %s", src.project)
	if err := adoGetJSON(adoURL(src, true, "_apis/git/repositories", query), description, &answer); err != nil {
		return nil, err
	}

	names := make([]string, 0, len(answer.Value))
	for _, repository := range answer.Value {
		if !repository.IsDisabled {
			names = append(names, repository.Name)
		}
	}
	return names, nil
}

// adoRoute returns the route that addresses itemPath in what source names: the organization,
// one of its projects, or a repository holding the item.
func adoRoute(src source, itemPath string) string {
	route := "/" + routeNamespace + "/" + kindADO + "/" + url.PathEscape(src.organization)
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
// isFolder tells whether a repository path names a folder, and is the repository's own answer
// unless a test gives another.
type adoTree struct {
	src      source
	isFolder func(source, string) bool
}

func (t adoTree) root() string { return "/" }

// folder returns the folder the addressed path lies in, which is the path itself when it
// names a folder rather than a document.
//
// A path ending in a slash, or carrying a Markdown suffix, says which it is on its own; any
// other is looked up, since a route reaching a folder is written without a trailing slash.
func (t adoTree) folder() string {
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

	var items []treeItem
	for _, found := range found {
		switch {
		case found.Path == "" || found.Path == folder || strings.HasPrefix(path.Base(found.Path), "."):
		case found.IsFolder:
			items = append(items, treeItem{path: found.Path, name: path.Base(found.Path), isFolder: true})
		case markdownExtensions[strings.ToLower(path.Ext(found.Path))]:
			items = append(items, treeItem{path: found.Path, name: path.Base(found.Path)})
		}
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

// readADODocument reads the document source addresses, resolving a folder to its index file.
//
// A folder is read the way a local directory is: the defaultCandidates are tried in order and
// the first one that exists is the document. A path that does not end in a slash is read
// directly, and looked up as a folder only if the repository says it is one.
func readADODocument(src source) (itemPath, content, objectID string, err error) {
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

	for _, candidate := range defaultCandidates {
		item, readErr := readADOItem(src, itemPath+candidate, true)
		if readErr != nil {
			continue
		}
		return itemPath + candidate, item.Content, item.ObjectID, nil
	}
	return "", "", "", fmt.Errorf("none of %s found in '%s'", strings.Join(defaultCandidates, ", "), itemPath)
}

// readADOSource reads the document source addresses in Azure DevOps.
//
// An organization and a project hold no document of their own, so each is read as a listing:
// of the organization's projects, and of the project's repositories. A repository is read as
// the file the path names.
func readADOSource(src source) (document, error) {
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

	itemPath, content, objectID, err := readADODocument(src)
	if err != nil {
		return document{}, err
	}
	name := itemPath
	if index := strings.LastIndex(name, "/"); index >= 0 {
		name = name[index+1:]
	}
	css, markdown := asMarkdownDocument(name, content)
	return document{marker: objectID, name: name, text: markdown, css: css}, nil
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

	text := strings.Join(lines, "\n") + "\n"
	return document{marker: textMarker(text), text: text}
}

// adoList returns the directory list beside a page of an Azure DevOps source.
//
// An organization holds the listing of its projects in the page itself, so it carries none. A
// project carries the organization's projects, the one on the page marked, since the page is
// the repositories below it. A repository carries its own documents, under the link back to
// the repositories of its project.
func adoList(src source, scope string) ([]listEntry, listLink) {
	switch {
	case src.project == "":
		return nil, listLink{}

	case src.repository == "":
		names, err := readADOProjects(src)
		if err != nil {
			logInfo("  %scannot list the projects of %s: %v%s", colorYellow, src.organization, err, colorReset)
			return nil, listLink{}
		}
		entries := make([]listEntry, 0, len(names))
		for _, name := range names {
			entries = append(entries, listEntry{Name: name, Current: name == src.project,
				Route: adoRoute(source{kind: kindADO, organization: src.organization, project: name}, "")})
		}
		sortEntries(entries)
		return entries, listLink{}
	}

	return documentTree(adoTree{src: src}, scope), adoUpLink(src)
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
