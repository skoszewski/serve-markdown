package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/git-pkgs/gitignore"
)

// ignoreFileName is the file of .gitignore patterns read from the served directory when
// --ignore-file names none.
const ignoreFileName = ".serveignore"

// ignoreRules holds .gitignore patterns relative to the served directory; the paths they
// match are left out of what is listed and served.
type ignoreRules struct {
	matcher *gitignore.Matcher
	// rootDir is the served directory, absolute and with its symbolic links resolved, that
	// ignoredPath reads a path relative to.
	rootDir string
}

// readIgnoreRules returns the rules of the patterns file holds, then of each list of extra
// patterns in turn, a later pattern overriding an earlier one. An empty file reads nothing,
// and a missing one fails with an error wrapping fs.ErrNotExist. A pattern that is not a
// .gitignore pattern fails with an error naming it.
func readIgnoreRules(file string, extra ...[]string) (*ignoreRules, error) {
	rules := &ignoreRules{matcher: gitignore.New("")}
	if file != "" {
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		rules.matcher.AddPatterns(content, "")
		if errs := rules.matcher.Errors(); len(errs) > 0 {
			return nil, fmt.Errorf("%s:%d: '%s' is not a .gitignore pattern (%s)", file, errs[0].Line, errs[0].Pattern, errs[0].Message)
		}
	}
	for _, patterns := range extra {
		rules.matcher.AddPatterns([]byte(strings.Join(patterns, "\n")), "")
		if errs := rules.matcher.Errors(); len(errs) > 0 {
			return nil, fmt.Errorf("'%s' is not a .gitignore pattern (%s)", errs[0].Pattern, errs[0].Message)
		}
	}
	return rules, nil
}

// ignored reports whether a slash-separated path relative to the served directory is matched
// by the rules, itself or through a folder it lies in; nil rules match nothing.
func (r *ignoreRules) ignored(relative string, isDir bool) bool {
	if r == nil || relative == "" || relative == "." {
		return false
	}
	return r.matcher.MatchPath(relative, isDir)
}

// ignoredPath reports whether a path below rootDir is matched by the rules; a path outside it
// is not.
func (r *ignoreRules) ignoredPath(file string, isDir bool) bool {
	if r == nil {
		return false
	}
	relative, err := filepath.Rel(r.rootDir, file)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return false
	}
	return r.ignored(filepath.ToSlash(relative), isDir)
}
