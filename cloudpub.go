package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type CloudPubManager struct {
	dataDir string
	mu      sync.Mutex
	running map[string]*exec.Cmd
}

func NewCloudPubManager(dataDir string) *CloudPubManager {
	return &CloudPubManager{dataDir: dataDir, running: map[string]*exec.Cmd{}}
}

func (m *CloudPubManager) Start(ctx context.Context, token, transferID, target string) (string, error) {
	clo, err := m.ensureCLI(ctx)
	if err != nil {
		return "", err
	}
	conf := filepath.Join(m.dataDir, "cloudpub", "config.toml")
	if err := os.MkdirAll(filepath.Dir(conf), 0o700); err != nil {
		return "", err
	}
	if err := exec.CommandContext(ctx, clo, "--conf", conf, "set", "token", strings.TrimSpace(token)).Run(); err != nil {
		return "", fmt.Errorf("cloudpub token setup failed: %w", err)
	}
	_ = os.Chmod(conf, 0o600)

	cmd := exec.Command(clo, "--conf", conf, "--log-level", "info", "publish", "-n", "DropOnce "+shortID(transferID), "http", target)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("cloudpub start failed: %w", err)
	}

	output := make(chan string, 32)
	go scanCloudPub(stdout, output)
	go scanCloudPub(stderr, output)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	timeout := time.NewTimer(25 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case chunk := <-output:
			if publicURL := cloudPubURL(chunk); publicURL != "" {
				m.mu.Lock()
				if old := m.running[transferID]; old != nil && old.Process != nil {
					_ = old.Process.Kill()
				}
				m.running[transferID] = cmd
				m.mu.Unlock()
				return publicURL, nil
			}
		case err := <-done:
			return "", fmt.Errorf("cloudpub stopped before publishing: %w", err)
		case <-timeout.C:
			_ = cmd.Process.Kill()
			return "", errors.New("cloudpub did not return public URL in time")
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			return "", ctx.Err()
		}
	}
}

func (m *CloudPubManager) Stop(transferID string) error {
	m.mu.Lock()
	cmd := m.running[transferID]
	delete(m.running, transferID)
	m.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}

func (m *CloudPubManager) StopAll() {
	m.mu.Lock()
	cmds := make([]*exec.Cmd, 0, len(m.running))
	for id, cmd := range m.running {
		cmds = append(cmds, cmd)
		delete(m.running, id)
	}
	m.mu.Unlock()
	for _, cmd := range cmds {
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
}

func scanCloudPub(reader io.Reader, output chan<- string) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)
	for scanner.Scan() {
		select {
		case output <- scanner.Text():
		default:
			// Start stops consuming output after it receives the public URL.
			// Keep draining the pipe so CloudPub cannot block on a full stderr/stdout buffer.
		}
	}
}

var cloudPubURLPattern = regexp.MustCompile(`https://[A-Za-z0-9.-]+(?::[0-9]+)?`)

func cloudPubURL(value string) string {
	return strings.TrimRight(cloudPubURLPattern.FindString(value), "/")
}

func shortID(value string) string {
	if len(value) <= 8 {
		return value
	}
	return value[:8]
}

func (m *CloudPubManager) ensureCLI(ctx context.Context) (string, error) {
	if path, err := exec.LookPath("clo"); err == nil {
		return path, nil
	}
	devPath := filepath.Join("tools", "cloudpub", "clo")
	if st, err := os.Stat(devPath); err == nil && !st.IsDir() {
		return devPath, nil
	}
	installDir := filepath.Join(m.dataDir, "cloudpub")
	cloPath := filepath.Join(installDir, "clo")
	if st, err := os.Stat(cloPath); err == nil && !st.IsDir() {
		return cloPath, nil
	}
	return "", errors.New("CloudPub CLI 'clo' was not found; install it and add it to PATH")
}
