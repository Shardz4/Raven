package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Issue holds the parsed details of a GitHub issue.
type Issue struct {
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	Labels   []string `json:"labels"`
	Language string   `json:"language"` // Primary repo language (e.g., "python", "go", "javascript")
	RepoURL  string   `json:"repo_url"`
	Owner    string   `json:"owner"`
	Repo     string   `json:"repo"`
	Number   int      `json:"number"`
	CloneURL string   `json:"clone_url"`
}

// Prompt builds a rich prompt string from the issue data for sending to LLMs.
func (i *Issue) Prompt() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("GitHub Issue #%d in %s/%s\n", i.Number, i.Owner, i.Repo))
	sb.WriteString(fmt.Sprintf("Title: %s\n", i.Title))
	if len(i.Labels) > 0 {
		sb.WriteString(fmt.Sprintf("Labels: %s\n", strings.Join(i.Labels, ", ")))
	}
	sb.WriteString(fmt.Sprintf("\n--- Issue Body ---\n%s\n", i.Body))
	return sb.String()
}

var issueURLPattern = regexp.MustCompile(`^https://github\.com/([^/]+)/([^/]+)/issues/(\d+)`)

// ParseIssueURL extracts owner, repo, and issue number from a GitHub issue URL.
func ParseIssueURL(rawURL string) (owner, repo string, number int, err error) {
	matches := issueURLPattern.FindStringSubmatch(rawURL)
	if matches == nil {
		return "", "", 0, fmt.Errorf("invalid GitHub issue URL: %s", rawURL)
	}
	owner = matches[1]
	repo = matches[2]
	_, err = fmt.Sscanf(matches[3], "%d", &number)
	return
}

// Fetcher retrieves GitHub issue data via the REST API.
type Fetcher struct {
	token  string
	client *http.Client
}

// NewFetcher creates a new GitHub issue fetcher. Token can be empty for public repos.
func NewFetcher(token string) *Fetcher {
	return &Fetcher{
		token: token,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// FetchIssue retrieves the full issue details from the GitHub API.
func (f *Fetcher) FetchIssue(issueURL string) (*Issue, error) {
	owner, repo, number, err := ParseIssueURL(issueURL)
	if err != nil {
		return nil, err
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d", owner, repo, number)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Raven-Agent/2.0")
	if f.token != "" {
		req.Header.Set("Authorization", "Bearer "+f.token)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned %d for %s", resp.StatusCode, apiURL)
	}

	var ghIssue struct {
		Title  string `json:"title"`
		Body   string `json:"body"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ghIssue); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	labels := make([]string, 0, len(ghIssue.Labels))
	for _, l := range ghIssue.Labels {
		labels = append(labels, l.Name)
	}

	// Detect repo language
	language := f.detectLanguage(owner, repo)

	return &Issue{
		Title:    ghIssue.Title,
		Body:     ghIssue.Body,
		Labels:   labels,
		Language: language,
		RepoURL:  fmt.Sprintf("https://github.com/%s/%s", owner, repo),
		Owner:    owner,
		Repo:     repo,
		Number:   number,
		CloneURL: fmt.Sprintf("https://github.com/%s/%s.git", owner, repo),
	}, nil
}

// detectLanguage queries the GitHub repo API for the primary language.
func (f *Fetcher) detectLanguage(owner, repo string) string {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "python" // default
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Raven-Agent/2.0")
	if f.token != "" {
		req.Header.Set("Authorization", "Bearer "+f.token)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return "python"
	}
	defer resp.Body.Close()

	var repoData struct {
		Language string `json:"language"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&repoData); err != nil {
		return "python"
	}

	if repoData.Language == "" {
		return "python"
	}
	return strings.ToLower(repoData.Language)
}
