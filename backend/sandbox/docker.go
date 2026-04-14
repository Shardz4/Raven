package sandbox

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// Result is the outcome of running a patch through the Docker sandbox.
type Result struct {
	Success    bool   `json:"success"`
	Logs       string `json:"logs"`
	ExitCode   int    `json:"exit_code"`
	DurationMs int64  `json:"duration_ms"`
}

// Manager manages Docker sandbox containers for patch verification.
type Manager struct {
	client    *client.Client
	imageTag  string
	timeoutS  int
}

// NewManager creates a new sandbox manager connected to the local Docker daemon.
func NewManager(imageTag string, timeoutSeconds int) (*Manager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &Manager{
		client:   cli,
		imageTag: imageTag,
		timeoutS: timeoutSeconds,
	}, nil
}

// EnsureImage builds the sandbox Docker image from the given path if needed.
func (m *Manager) EnsureImage(dockerfilePath string) error {
	ctx := context.Background()

	// Check if image already exists
	images, err := m.client.ImageList(ctx, image.ListOptions{})
	if err == nil {
		for _, img := range images {
			for _, tag := range img.RepoTags {
				if tag == m.imageTag {
					log.Printf("[sandbox] Image %s already exists", m.imageTag)
					return nil
				}
			}
		}
	}

	log.Printf("[sandbox] Building image %s from %s...", m.imageTag, dockerfilePath)
	return fmt.Errorf("sandbox image %s not found — run 'docker build -t %s %s' manually", m.imageTag, m.imageTag, dockerfilePath)
}

// RunVerification spins up a container, injects the patch code and test script,
// then executes the test script and returns the result.
// This fixes the race condition from the Python version by using `docker exec`
// instead of running the command at container creation time.
func (m *Manager) RunVerification(code, testScript string) (*Result, error) {
	ctx := context.Background()
	start := time.Now()

	// Create the container in a paused state (just `sleep`)
	resp, err := m.client.ContainerCreate(ctx, &container.Config{
		Image: m.imageTag,
		Cmd:   []string{"sleep", fmt.Sprintf("%d", m.timeoutS+10)},
	}, &container.HostConfig{
		Resources: container.Resources{
			Memory:   512 * 1024 * 1024, // 512 MB
			PidsLimit: int64Ptr(128),
		},
	}, nil, nil, "")
	if err != nil {
		return &Result{Success: false, Logs: fmt.Sprintf("container create: %v", err)}, nil
	}
	containerID := resp.ID

	// Always clean up
	defer func() {
		_ = m.client.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
	}()

	// Start the container
	if err := m.client.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return &Result{Success: false, Logs: fmt.Sprintf("container start: %v", err)}, nil
	}

	// Inject files into /app/ BEFORE executing anything
	if err := m.copyToContainer(ctx, containerID, "solution.py", code); err != nil {
		return &Result{Success: false, Logs: fmt.Sprintf("inject solution.py: %v", err)}, nil
	}
	if err := m.copyToContainer(ctx, containerID, "run_tests.sh", testScript); err != nil {
		return &Result{Success: false, Logs: fmt.Sprintf("inject run_tests.sh: %v", err)}, nil
	}

	// Now exec the test script inside the running container
	execCfg := container.ExecOptions{
		Cmd:          []string{"/bin/bash", "/app/run_tests.sh"},
		AttachStdout: true,
		AttachStderr: true,
	}
	execResp, err := m.client.ContainerExecCreate(ctx, containerID, execCfg)
	if err != nil {
		return &Result{Success: false, Logs: fmt.Sprintf("exec create: %v", err)}, nil
	}

	// Attach to the exec to get stdout/stderr
	attachResp, err := m.client.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{})
	if err != nil {
		return &Result{Success: false, Logs: fmt.Sprintf("exec attach: %v", err)}, nil
	}
	defer attachResp.Close()

	// Read output with timeout
	var stdout, stderr bytes.Buffer
	doneCh := make(chan error, 1)
	go func() {
		_, err := stdcopy.StdCopy(&stdout, &stderr, attachResp.Reader)
		doneCh <- err
	}()

	timeout := time.Duration(m.timeoutS) * time.Second
	select {
	case <-doneCh:
	case <-time.After(timeout):
		return &Result{
			Success:    false,
			Logs:       fmt.Sprintf("Sandbox timeout after %ds\nStdout: %s\nStderr: %s", m.timeoutS, stdout.String(), stderr.String()),
			ExitCode:   -1,
			DurationMs: time.Since(start).Milliseconds(),
		}, nil
	}

	// Get the exit code
	inspectResp, err := m.client.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return &Result{Success: false, Logs: fmt.Sprintf("exec inspect: %v", err)}, nil
	}

	logs := stdout.String()
	if stderr.Len() > 0 {
		logs += "\n--- stderr ---\n" + stderr.String()
	}

	return &Result{
		Success:    inspectResp.ExitCode == 0,
		Logs:       logs,
		ExitCode:   inspectResp.ExitCode,
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}

// copyToContainer injects an in-memory file into a running container.
func (m *Manager) copyToContainer(ctx context.Context, containerID, filename, content string) error {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	data := []byte(content)
	hdr := &tar.Header{
		Name: filename,
		Mode: 0755,
		Size: int64(len(data)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := tw.Write(data); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}

	return m.client.CopyToContainer(ctx, containerID, "/app", &buf, container.CopyToContainerOptions{})
}

// BuildTestScript generates the bash script that clones the repo and runs tests (Python).
func BuildTestScript(cloneURL string) string {
	return BuildTestScriptForLanguage(cloneURL, "python")
}

// BuildTestScriptForLanguage generates a language-aware test script for sandbox verification.
func BuildTestScriptForLanguage(cloneURL, language string) string {
	switch language {
	case "go", "golang":
		return fmt.Sprintf(`#!/bin/bash
set -e
echo "=== Raven Sandbox Verification (Go) ==="
echo "Cloning: %s"
git clone --depth 1 %s target_repo || exit 1
cd target_repo
echo "Applying AI-generated patch..."
cp /app/solution.go .
echo "Running Go tests..."
go test ./... -v -count=1 2>&1
echo "=== Verification Complete ==="
`, cloneURL, cloneURL)

	case "javascript", "typescript", "js", "ts":
		return fmt.Sprintf(`#!/bin/bash
set -e
echo "=== Raven Sandbox Verification (JavaScript) ==="
echo "Cloning: %s"
git clone --depth 1 %s target_repo || exit 1
cd target_repo
echo "Applying AI-generated patch..."
cp /app/solution.js .
echo "Installing dependencies..."
if [ -f package.json ]; then npm install --silent; fi
echo "Running tests..."
if [ -f package.json ]; then npm test 2>&1; else node solution.js; fi
echo "=== Verification Complete ==="
`, cloneURL, cloneURL)

	case "rust":
		return fmt.Sprintf(`#!/bin/bash
set -e
echo "=== Raven Sandbox Verification (Rust) ==="
echo "Cloning: %s"
git clone --depth 1 %s target_repo || exit 1
cd target_repo
echo "Applying AI-generated patch..."
cp /app/solution.rs src/
echo "Running Rust tests..."
cargo test 2>&1
echo "=== Verification Complete ==="
`, cloneURL, cloneURL)

	default: // python
		return fmt.Sprintf(`#!/bin/bash
set -e
echo "=== Raven Sandbox Verification (Python) ==="
echo "Cloning: %s"
git clone --depth 1 %s target_repo || exit 1
cd target_repo
echo "Applying AI-generated patch..."
cp /app/solution.py .
echo "Installing dependencies..."
if [ -f requirements.txt ]; then pip install -q -r requirements.txt; fi
echo "Running tests..."
python -m pytest -q --tb=short 2>&1
echo "=== Verification Complete ==="
`, cloneURL, cloneURL)
	}
}

// Close releases the Docker client resources.
func (m *Manager) Close() error {
	return m.client.Close()
}

func int64Ptr(v int64) *int64 {
	return &v
}

// Ensure io is used (for stdcopy)
var _ io.Reader
