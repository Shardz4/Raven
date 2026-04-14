package github

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// PRRequest contains the data needed to open a PR.
type PRRequest struct {
	Owner       string
	Repo        string
	IssueNumber int
	Title       string
	Body        string
	PatchCode   string
	BranchName  string
}

// PRResult is the outcome of the auto-PR process.
type PRResult struct {
	PRURL    string `json:"pr_url"`
	PRNumber int    `json:"pr_number"`
	Branch   string `json:"branch"`
}

// PRCreator creates pull requests via the GitHub API.
type PRCreator struct {
	token  string
	client *http.Client
}

// NewPRCreator creates a new PR creator. Token is REQUIRED for creating PRs.
func NewPRCreator(token string) *PRCreator {
	return &PRCreator{
		token: token,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CanCreatePR returns true if a GitHub token is configured.
func (p *PRCreator) CanCreatePR() bool {
	return p.token != ""
}

// CreatePR forks the repo (if needed), creates a branch with the patch,
// and opens a pull request against the original repo.
func (p *PRCreator) CreatePR(req *PRRequest) (*PRResult, error) {
	if p.token == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN required for auto-PR")
	}

	// 1. Fork the repo (GitHub auto-deduplicates — safe to call multiple times)
	forkOwner, err := p.forkRepo(req.Owner, req.Repo)
	if err != nil {
		return nil, fmt.Errorf("fork: %w", err)
	}

	// 2. Get the default branch SHA
	baseSHA, defaultBranch, err := p.getDefaultBranchSHA(req.Owner, req.Repo)
	if err != nil {
		return nil, fmt.Errorf("get base SHA: %w", err)
	}

	// 3. Create a new branch in our fork
	branchName := req.BranchName
	if branchName == "" {
		branchName = fmt.Sprintf("raven/fix-issue-%d", req.IssueNumber)
	}
	if err := p.createBranch(forkOwner, req.Repo, branchName, baseSHA); err != nil {
		return nil, fmt.Errorf("create branch: %w", err)
	}

	// 4. Commit the patch file to the branch
	commitMsg := fmt.Sprintf("fix: resolve issue #%d via Raven AI\n\nAutomatically generated and verified by RavenMind consensus.", req.IssueNumber)
	if err := p.commitFile(forkOwner, req.Repo, branchName, "solution.py", req.PatchCode, commitMsg); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	// 5. Open a PR from our fork to the original repo
	prNumber, prURL, err := p.openPR(req.Owner, req.Repo, forkOwner, branchName, defaultBranch, req.Title, req.Body)
	if err != nil {
		return nil, fmt.Errorf("open PR: %w", err)
	}

	return &PRResult{
		PRURL:    prURL,
		PRNumber: prNumber,
		Branch:   branchName,
	}, nil
}

func (p *PRCreator) ghAPI(method, path string, body any) (map[string]any, error) {
	var reqBody *bytes.Buffer
	if body != nil {
		data, _ := json.Marshal(body)
		reqBody = bytes.NewBuffer(data)
	} else {
		reqBody = &bytes.Buffer{}
	}

	url := "https://api.github.com" + path
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Raven-Agent/2.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	if resp.StatusCode >= 400 {
		msg := ""
		if m, ok := result["message"]; ok {
			msg = fmt.Sprintf("%v", m)
		}
		return result, fmt.Errorf("GitHub API %s %s returned %d: %s", method, path, resp.StatusCode, msg)
	}
	return result, nil
}

func (p *PRCreator) forkRepo(owner, repo string) (string, error) {
	result, err := p.ghAPI("POST", fmt.Sprintf("/repos/%s/%s/forks", owner, repo), map[string]any{})
	if err != nil {
		// 202 Accepted is normal for forks
		if result != nil {
			if ownerObj, ok := result["owner"].(map[string]any); ok {
				if login, ok := ownerObj["login"].(string); ok {
					return login, nil
				}
			}
		}
		return "", err
	}
	if ownerObj, ok := result["owner"].(map[string]any); ok {
		if login, ok := ownerObj["login"].(string); ok {
			return login, nil
		}
	}
	return "", fmt.Errorf("could not determine fork owner")
}

func (p *PRCreator) getDefaultBranchSHA(owner, repo string) (sha, branch string, err error) {
	result, err := p.ghAPI("GET", fmt.Sprintf("/repos/%s/%s", owner, repo), nil)
	if err != nil {
		return "", "", err
	}
	branch, _ = result["default_branch"].(string)
	if branch == "" {
		branch = "main"
	}

	// Get the SHA of the branch tip
	refResult, err := p.ghAPI("GET", fmt.Sprintf("/repos/%s/%s/git/ref/heads/%s", owner, repo, branch), nil)
	if err != nil {
		return "", "", err
	}
	if obj, ok := refResult["object"].(map[string]any); ok {
		sha, _ = obj["sha"].(string)
	}
	return sha, branch, nil
}

func (p *PRCreator) createBranch(owner, repo, branchName, sha string) error {
	_, err := p.ghAPI("POST", fmt.Sprintf("/repos/%s/%s/git/refs", owner, repo), map[string]any{
		"ref": "refs/heads/" + branchName,
		"sha": sha,
	})
	return err
}

func (p *PRCreator) commitFile(owner, repo, branch, filePath, content, message string) error {
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	_, err := p.ghAPI("PUT", fmt.Sprintf("/repos/%s/%s/contents/%s", owner, repo, filePath), map[string]any{
		"message": message,
		"content": encoded,
		"branch":  branch,
	})
	return err
}

func (p *PRCreator) openPR(upstreamOwner, repo, forkOwner, branchName, baseBranch, title, body string) (int, string, error) {
	result, err := p.ghAPI("POST", fmt.Sprintf("/repos/%s/%s/pulls", upstreamOwner, repo), map[string]any{
		"title": title,
		"body":  body,
		"head":  fmt.Sprintf("%s:%s", forkOwner, branchName),
		"base":  baseBranch,
	})
	if err != nil {
		return 0, "", err
	}
	number := 0
	if n, ok := result["number"].(float64); ok {
		number = int(n)
	}
	url, _ := result["html_url"].(string)
	return number, url, nil
}
