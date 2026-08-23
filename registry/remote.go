// Package registry is where mods come from.
//
// A remote is a git repository that follows the Sacred Repository Mod Layout:
// one file at its root, `sacred.mods.repository.json`, saying what it holds and
// where each mod's jar can be downloaded. Anybody can host one, the launcher
// starts with ours, and adding a second is pasting a clone URL.
//
// Nothing here runs git. What the launcher needs out of a repository is one
// small file and some icons, and cloning a repository that is encouraged to
// carry its mods' source as well would be tens of megabytes for a few kilobytes
// of answer. The clone URL is an address, and this turns it into the address of
// a file.
package registry

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// Official is the remote a launcher has before a player adds anything. It is a
// list because a second one is a partner, not a rewrite.
var Official = []string{"https://github.com/ancaria-dev/mods.git"}

// Index is the file that makes a repository a mod repository.
const IndexFile = "sacred.mods.repository.json"

// Remote is one repository of mods, as the launcher remembers it.
type Remote struct {
	// URL is the clone URL, `.git` and all, the way a player pasted it. It is
	// what identifies the remote and what is shown under a mod's name.
	URL string `json:"url"`

	// Raw is a template with `{path}` in it, resolved once when the remote was
	// added and kept because working it out again means asking a server which
	// of several URL shapes it speaks.
	Raw string `json:"raw"`

	// Name comes from the index. Only for the list; the URL is the identity.
	Name string `json:"name"`

	// Token is an access token for a repository that is not public. Sealed on
	// its way into the settings file, plain in memory here.
	Token string `json:"token"`
}

// Label is what a mod's "from" line says: the owner and repository, since the
// scheme and the .git are the same on every row and only cost width.
func (r Remote) Label() string {
	if trimmed := strings.TrimSuffix(r.URL, ".git"); trimmed != r.URL {
		if parsed, err := url.Parse(trimmed); err == nil {
			return parsed.Host + strings.TrimSuffix(parsed.Path, "/")
		}
	}
	return r.URL
}

// File is the URL of one path inside the repository.
func (r Remote) File(path string) string {
	return strings.ReplaceAll(r.Raw, "{path}", path)
}

// Header is what every request to this remote carries.
func (r Remote) Header() http.Header {
	head := http.Header{}
	if r.Token == "" {
		return head
	}
	head.Set("Authorization", "Bearer "+r.Token)
	// GitHub's raw host does not take a token at all; its API does, and answers
	// with the file itself only when asked to.
	if strings.Contains(r.Raw, "api.github.com") {
		head.Set("Accept", "application/vnd.github.raw")
	}
	return head
}

// Normalize checks a pasted address and gives it back in one spelling.
//
// The `.git` suffix is required, and that is deliberate rather than technical:
// it is the difference between a URL somebody meant to paste and a URL they had
// in the clipboard. Every forge offers exactly this string on the button
// labelled Code.
func Normalize(address string) (string, error) {
	trimmed := strings.TrimSpace(address)
	trimmed = strings.TrimSuffix(trimmed, "/")
	if trimmed == "" {
		return "", errors.New("Paste the URL of a mod repository")
	}
	if strings.HasPrefix(trimmed, "git@") || strings.HasPrefix(trimmed, "ssh://") {
		return "", errors.New("This is an SSH URL. Use the HTTPS URL copied from " +
			"the same Code menu")
	}
	if !strings.HasPrefix(trimmed, "https://") {
		return "", errors.New("The repository URL must start with https://")
	}
	if !strings.HasSuffix(trimmed, ".git") {
		return "", errors.New("The repository URL must end in .git. Copy the clone " +
			"URL from the Code menu, not the page URL")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" || strings.Count(strings.Trim(parsed.Path, "/"), "/") < 1 {
		return "", errors.New("This does not look like a repository URL")
	}
	return trimmed, nil
}

// Candidates is every URL shape worth trying for a repository, best first.
//
// Forges do not agree on how to serve one file out of a repository, and there
// is no way to ask. So the shapes are tried once, when the remote is added, and
// the one that answered with a readable index is what gets remembered. Three
// requests once beats a table of hostnames that goes stale.
func Candidates(clone string) []string {
	trimmed := strings.TrimSuffix(clone, ".git")
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil
	}
	host := parsed.Host
	path := strings.Trim(parsed.Path, "/")

	if host == "github.com" {
		return []string{
			// Public. HEAD rather than a branch name: it resolves to whichever
			// one the repository calls its default.
			"https://raw.githubusercontent.com/" + path + "/HEAD/{path}",
			// Private, with a token. The raw host does not accept one.
			"https://api.github.com/repos/" + path + "/contents/{path}",
		}
	}
	return []string{
		// GitLab, and anything that copied it.
		"https://" + host + "/" + path + "/-/raw/HEAD/{path}",
		// Gitea and Forgejo, which want a branch and have no HEAD alias.
		"https://" + host + "/" + path + "/raw/branch/main/{path}",
		"https://" + host + "/" + path + "/raw/branch/master/{path}",
	}
}
