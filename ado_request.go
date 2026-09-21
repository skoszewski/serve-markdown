package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// adoRequestError reports that an Azure DevOps REST request failed, carrying the API's own
// error message. It is told apart from other failures so that a path which cannot be read can
// still be looked up as a folder.
type adoRequestError struct {
	message string
}

func (e *adoRequestError) Error() string { return e.message }

// adoURL builds the URL of an Azure DevOps REST endpoint below the organization source names.
//
// endpoint is the path after the organization and, when project is asked for, after the
// project as well; query carries the parameters, api-version among them.
func adoURL(src source, withProject bool, endpoint string, query url.Values) string {
	address := "https://dev.azure.com/" + url.PathEscape(src.organization)
	if withProject {
		address += "/" + url.PathEscape(src.project)
	}
	return fmt.Sprintf("%s/%s?%s", address, endpoint, query.Encode())
}

// adoItemsURL builds the URL of the items endpoint of the repository source names.
func adoItemsURL(src source, query url.Values) string {
	return adoURL(src, true, "_apis/git/repositories/"+url.PathEscape(src.repository)+"/items", query)
}

// adoGet performs one Azure DevOps REST request and returns the body it answered with.
//
// description names what was being read, for the error a failure raises. accept is the media
// type asked for, since the API answers with the item itself as readily as with JSON.
func adoGet(requestURL, accept, description string) ([]byte, error) {
	token, err := accessTokenFromAZ()
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", accept)

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

// adoGetJSON performs one Azure DevOps REST request and reads its answer into answer.
func adoGetJSON(requestURL, description string, answer any) error {
	body, err := adoGet(requestURL, "application/json", description)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, answer); err != nil {
		return fmt.Errorf("%s did not answer with JSON", description)
	}
	return nil
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
