package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"path"
	"strings"
	"sync"
	"time"
)

// Azure DevOps AAD resource ID used to acquire access tokens for the REST API.
const adoResourceID = "499b84ac-1321-427f-aa17-267ca6975798"

const adoGitAPIVersion = "7.1"

const adoTokenLifetime = 1800 * time.Second

// adoRequestError reports that an Azure DevOps REST request failed, carrying the API's own
// error message. It is told apart from other failures so that a path which cannot be read can
// still be looked up as a folder.
type adoRequestError struct {
	message string
}

func (e *adoRequestError) Error() string { return e.message }

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

// parseADOLocation splits "<organization>/<project>/<repository>/<path to file>" into an ado
// source.
//
// A location naming only a repository, or ending in a slash, addresses a folder, whose index
// file is looked up the way a local directory's is. The second result is false when location
// does not name at least a repository.
func parseADOLocation(location string) (source, bool) {
	var parts []string
	for _, part := range strings.Split(unescapePath(location), "/") {
		if part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) < 3 {
		return source{}, false
	}

	path := "/" + strings.Join(parts[3:], "/")
	if len(parts) == 3 || strings.HasSuffix(location, "/") {
		path = strings.TrimSuffix(path, "/") + "/"
	}
	return source{
		kind:         kindADO,
		organization: parts[0],
		project:      parts[1],
		repository:   parts[2],
		path:         path,
	}, true
}

// parseADOURL splits an ado:// URL into the source it names, failing if it is incomplete.
func parseADOURL(rawURL string) (source, error) {
	src, ok := parseADOLocation(strings.TrimPrefix(rawURL, adoScheme))
	if !ok {
		return source{}, fmt.Errorf("'%s' is not a complete %s URL; expected %s<organization>/<project>/<repository>[/<path to file>]",
			rawURL, adoScheme, adoScheme)
	}
	return src, nil
}

// adoWebURL returns the Azure DevOps web URL showing the file source names.
func adoWebURL(src source) string {
	// The path keeps its separators, the way Azure DevOps writes it in a browser's address bar.
	segments := strings.Split(src.path, "/")
	for index, segment := range segments {
		segments[index] = url.PathEscape(segment)
	}
	return fmt.Sprintf("https://dev.azure.com/%s/%s/_git/%s?path=%s",
		url.PathEscape(src.organization), url.PathEscape(src.project),
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
	token, err := accessTokenFromAZ()
	if err != nil {
		return item, err
	}

	query := url.Values{}
	query.Set("path", itemPath)
	query.Set("includeContent", fmt.Sprintf("%t", includeContent))
	query.Set("includeContentMetadata", "true")
	query.Set("api-version", adoGitAPIVersion)
	requestURL := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories/%s/items?$format=json&%s",
		url.PathEscape(src.organization), url.PathEscape(src.project),
		url.PathEscape(src.repository), query.Encode())

	description := fmt.Sprintf("reading %s from %s", itemPath, src.repository)

	request, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return item, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return item, &adoRequestError{fmt.Sprintf("%s failed: %v", description, err)}
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return item, &adoRequestError{fmt.Sprintf("%s failed: %v", description, err)}
	}
	if response.StatusCode != http.StatusOK {
		return item, &adoRequestError{fmt.Sprintf("%s failed: %d %s", description,
			response.StatusCode, adoErrorMessage(body, response.Status))}
	}
	if err := json.Unmarshal(body, &item); err != nil {
		answeredWith := response.Header.Get("Content-Type")
		if answeredWith == "" {
			answeredWith = "no content type"
		}
		return item, fmt.Errorf("%s answered with %s instead of JSON", description, answeredWith)
	}
	return item, nil
}

// readADORawItem reads the file source addresses as the bytes it holds, for a picture or
// another file that is not text.
//
// The item itself is read rather than JSON holding its content, and a Git LFS pointer is
// resolved to the file it stands for.
func readADORawItem(src source) ([]byte, error) {
	token, err := accessTokenFromAZ()
	if err != nil {
		return nil, err
	}

	query := url.Values{}
	query.Set("path", src.path)
	query.Set("resolveLfs", "true")
	query.Set("api-version", adoGitAPIVersion)
	requestURL := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories/%s/items?%s",
		url.PathEscape(src.organization), url.PathEscape(src.project),
		url.PathEscape(src.repository), query.Encode())

	description := fmt.Sprintf("reading %s from %s", src.path, src.repository)

	request, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/octet-stream")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, &adoRequestError{fmt.Sprintf("%s failed: %v", description, err)}
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, &adoRequestError{fmt.Sprintf("%s failed: %v", description, err)}
	}
	if response.StatusCode != http.StatusOK {
		return nil, &adoRequestError{fmt.Sprintf("%s failed: %d %s", description,
			response.StatusCode, adoErrorMessage(body, response.Status))}
	}
	return body, nil
}

// readADOItems lists the items below folder in the repository source names, one level down or
// the whole tree under it.
func readADOItems(src source, folder string, recursive bool) ([]gitItem, error) {
	token, err := accessTokenFromAZ()
	if err != nil {
		return nil, err
	}

	recursion := "OneLevel"
	if recursive {
		recursion = "Full"
	}
	query := url.Values{}
	query.Set("scopePath", folder)
	query.Set("recursionLevel", recursion)
	query.Set("api-version", adoGitAPIVersion)
	requestURL := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories/%s/items?$format=json&%s",
		url.PathEscape(src.organization), url.PathEscape(src.project),
		url.PathEscape(src.repository), query.Encode())

	description := fmt.Sprintf("listing %s in %s", folder, src.repository)

	request, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, &adoRequestError{fmt.Sprintf("%s failed: %v", description, err)}
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, &adoRequestError{fmt.Sprintf("%s failed: %v", description, err)}
	}
	if response.StatusCode != http.StatusOK {
		return nil, &adoRequestError{fmt.Sprintf("%s failed: %d %s", description,
			response.StatusCode, adoErrorMessage(body, response.Status))}
	}

	var answer struct {
		Value []gitItem `json:"value"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		return nil, fmt.Errorf("%s did not answer with JSON", description)
	}
	return answer.Value, nil
}

// adoRoute returns the route that addresses itemPath in the repository source names.
func adoRoute(src source, itemPath string) string {
	return fmt.Sprintf("/%s/%s/%s/%s/%s%s", routeNamespace, kindADO,
		url.PathEscape(src.organization), url.PathEscape(src.project),
		url.PathEscape(src.repository), escapeRoute(itemPath))
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

// adoErrorMessage extracts the Azure DevOps error text from a failed response body, falling
// back to the HTTP status.
func adoErrorMessage(body []byte, status string) string {
	var answer struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &answer); err == nil && answer.Message != "" {
		return answer.Message
	}
	return status
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
