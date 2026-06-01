package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client implements Storer by making HTTP requests to a remote Store Service.
type Client struct {
	baseURL string
	client  *http.Client
}

// NewClient creates a new Store Service client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) CreateJob(job *Job) error {
	data, err := json.Marshal(job)
	if err != nil {
		return err
	}

	resp, err := c.client.Post(c.baseURL+"/api/jobs", "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}
	return nil
}

func (c *Client) UpdateJobResult(job *Job) error {
	data, err := json.Marshal(job)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPut, c.baseURL+"/api/jobs", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}
	return nil
}

func (c *Client) GetJob(id string) (*Job, error) {
	resp, err := c.client.Get(fmt.Sprintf("%s/api/jobs/%s", c.baseURL, id))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("job not found")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var job Job
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return nil, err
	}
	return &job, nil
}

func (c *Client) ListJobs(limit int) ([]*Job, error) {
	resp, err := c.client.Get(fmt.Sprintf("%s/api/jobs?limit=%d", c.baseURL, limit))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var jobs []*Job
	if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

func (c *Client) RecordResult(model string, won bool, score float64) error {
	payload := map[string]any{
		"model": model,
		"won":   won,
		"score": score,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := c.client.Post(c.baseURL+"/api/leaderboard", "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}
	return nil
}

func (c *Client) GetLeaderboard() ([]*LeaderboardEntry, error) {
	resp, err := c.client.Get(c.baseURL + "/api/leaderboard")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var entries []*LeaderboardEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (c *Client) Close() error {
	return nil
}
