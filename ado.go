package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
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
