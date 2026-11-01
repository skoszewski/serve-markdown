package render

import (
	"fmt"
	"hash/fnv"
	"mime"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// MarkdownExtensions are the suffixes of the files read as Markdown.
var MarkdownExtensions = map[string]bool{".md": true, ".markdown": true}

var codeFenceLanguages = map[string]string{
	".ps1":    "powershell",
	".py":     "python",
	".rb":     "ruby",
	".sh":     "bash",
	".tf":     "hcl",
	".tfvars": "hcl",
	".txt":    "plaintext",
	".yml":    "yaml",
	".zsh":    "bash",
}

// FrontMatterPattern matches a front matter block: it opens on the file's first line and
// closes on a "---" or "..." line, the block's text being its first group.
var FrontMatterPattern = regexp.MustCompile(`(?s)\A---[ \t]*\r?\n((?:.*?\r?\n)??)(?:---|\.\.\.)[ \t]*(?:\r?\n|\z)`)

var backtickRunPattern = regexp.MustCompile("`+")

// Files with these suffixes are served as the bytes they hold rather than read as documents.
var rawAssetExtensions = map[string]bool{
	".avif": true, ".bmp": true, ".gif": true, ".ico": true, ".jpeg": true,
	".jpg": true, ".pdf": true, ".png": true, ".svg": true, ".webp": true,
}

// IsRawAsset reports whether name names a file served as its own bytes.
func IsRawAsset(name string) bool {
	return rawAssetExtensions[strings.ToLower(path.Ext(name))]
}

// ContentType returns the media type name's suffix names, falling back to a byte stream.
func ContentType(name string) string {
	if mediaType := mime.TypeByExtension(strings.ToLower(path.Ext(name))); mediaType != "" {
		return mediaType
	}
	return "application/octet-stream"
}

// DocumentInfo is what a document's front matter says of it: its title, author and date, shown
// in the page's top row, and the page it is printed on as a PDF, which stays on the server.
type DocumentInfo struct {
	Title  string            `json:"title"`
	Author string            `json:"author"`
	Date   string            `json:"date"`
	PDF    map[string]string `json:"-"`
}

// Content is the JSON the page fetches its document as. Its mtime, text, css, base and info are
// null when the document could not be read, and error says why.
//
// mtime marks the version read, so that the page renders again only once it differs. base is
// the route the document's relative links are resolved against, which the page cannot work out
// for itself: a route naming a folder resolves to a document inside it, and the browser would
// resolve the links beside the folder instead.
type Content struct {
	MTime *string       `json:"mtime"`
	Text  *string       `json:"text"`
	CSS   *string       `json:"css"`
	Base  *string       `json:"base"`
	Info  *DocumentInfo `json:"info"`
	Error *string       `json:"error"`
}

// FrontMatter splits a Markdown document's front matter off and returns the CSS it declares and
// what it says of the document, with the body; a document without front matter is returned
// whole, and one with it returned shorter.
//
// The front matter is dropped from the document, since Markdown renderers show it as a rule
// followed by its raw keys. Its "css" key styles the page, its "title", "author" and "date"
// keys describe the document, and its "pdf" mapping names the page it is printed on; a key that
// is not a single value is not read, and a block that is not a mapping leaves the document with
// the server's own styling alone. The error tells of a block that does not parse as YAML.
func FrontMatter(text string) (css string, info DocumentInfo, body string, err error) {
	match := FrontMatterPattern.FindStringSubmatchIndex(text)
	if match == nil {
		return "", DocumentInfo{}, text, nil
	}

	body = text[match[1]:]
	var frontMatter any
	if err := yaml.Unmarshal([]byte(text[match[2]:match[3]]), &frontMatter); err != nil {
		return "", DocumentInfo{}, body, err
	}
	if mapping, isMapping := frontMatter.(map[string]any); isMapping {
		if declared, isString := mapping["css"].(string); isString {
			css = declared
		}
		info = DocumentInfo{Title: FrontMatterValue(mapping["title"]),
			Author: FrontMatterValue(mapping["author"]), Date: FrontMatterValue(mapping["date"])}
		if page, isMapping := mapping["pdf"].(map[string]any); isMapping {
			info.PDF = map[string]string{}
			for key, value := range page {
				info.PDF[key] = FrontMatterValue(value)
			}
		}
	}
	return css, info, body, nil
}

// FrontMatterValue returns a single front matter value as the text it is shown as, a date
// written without a time as the date alone, and nothing for a list, a mapping or no value.
func FrontMatterValue(value any) string {
	switch value := value.(type) {
	case string:
		return strings.TrimSpace(value)
	case time.Time:
		if hour, minute, second := value.Clock(); hour+minute+second+value.Nanosecond() == 0 {
			return value.Format(time.DateOnly)
		}
		return value.Format("2006-01-02 15:04")
	case int, float64, bool:
		return fmt.Sprint(value)
	}
	return ""
}

// CodeDocument returns the Markdown document showing the file name holds as a code block,
// titled by its name and highlighted in the language its suffix names.
//
// The code fence is made longer than the longest run of backticks in the file, so a file that
// itself contains fences stays inside its own block.
func CodeDocument(name, text string) string {
	suffix := strings.ToLower(path.Ext(name))
	language, known := codeFenceLanguages[suffix]
	if !known {
		language = strings.TrimPrefix(suffix, ".")
	}

	longest := 0
	for _, run := range backtickRunPattern.FindAllString(text, -1) {
		if len(run) > longest {
			longest = len(run)
		}
	}
	fence := strings.Repeat("`", max(3, longest+1))
	return fmt.Sprintf("# %s\n\n%s%s\n%s\n%s\n", name, fence, language, strings.TrimRight(text, "\n"), fence)
}

// DirectoryListing returns the Markdown document titled title listing names as the files a
// folder holds, or saying that it holds none.
func DirectoryListing(title string, names []string) string {
	lines := []string{"# " + title, ""}
	if len(names) == 0 {
		lines = append(lines, "No Markdown files found.")
	}
	for _, name := range names {
		lines = append(lines, fmt.Sprintf("- [%s](%s)", name, name))
	}
	return strings.Join(lines, "\n") + "\n"
}

// DocumentBase returns the route relative links in the document are resolved against: the
// route of the folder holding it, ending in a slash.
//
// name is the file the document was read from; a route ending in it names the document itself,
// and any other route names the folder it lies in.
func DocumentBase(route, name string) string {
	if route == "" {
		route = "/"
	}
	if name != "" && UnescapePath(path.Base(route)) == name {
		route = path.Dir(route)
	}
	return strings.TrimSuffix(route, "/") + "/"
}

// TextMarker returns the change marker of a document without a modification time or an object
// ID of its own, made from the text it holds.
func TextMarker(text string) string {
	sum := fnv.New64a()
	sum.Write([]byte(text))
	return strconv.FormatUint(sum.Sum64(), 16)
}

// UnescapePath decodes the percent escapes in a URL path, leaving it unchanged when it holds an
// invalid escape.
func UnescapePath(route string) string {
	decoded, err := url.PathUnescape(route)
	if err != nil {
		return route
	}
	return decoded
}

// EscapeRoute escapes each segment of a slash separated path, for a route that addresses it.
func EscapeRoute(name string) string {
	segments := strings.Split(name, "/")
	for index, segment := range segments {
		segments[index] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}
