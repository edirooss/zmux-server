package services

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"go.uber.org/zap"
)

// SystemdService handles systemd operations
type SystemdService struct {
	unitTemplate *template.Template
}

// NewSystemdService creates a new systemd service handler
func NewSystemdService(_ *zap.Logger) (*SystemdService, error) {
	tmpl, err := template.ParseFiles(templateFilePath())
	if err != nil {
		return nil, fmt.Errorf("failed to parse systemd template: %w", err)
	}
	return &SystemdService{unitTemplate: tmpl}, nil
}

// SystemdServiceConfig represents the configuration for a systemd service
type SystemdServiceConfig struct {
	ServiceName string
	ExecStart   string
	RestartSec  string
}

// CommitService saves a systemd service
func (s *SystemdService) CommitService(cfg SystemdServiceConfig) error {
	serviceFilePath := filepath.Join("/etc/systemd/system", cfg.ServiceName+".service")

	// Escape % characters for systemd
	cfg.ExecStart = strings.ReplaceAll(cfg.ExecStart, "%", "%%")

	// Create service file
	file, err := os.Create(serviceFilePath)
	if err != nil {
		return fmt.Errorf("create service file: %w", err)
	}

	// Write from template
	if err := s.unitTemplate.Execute(file, cfg); err != nil {
		file.Close()
		_ = os.Remove(serviceFilePath) // best-effort cleanup
		return fmt.Errorf("execute template: %w", err)
	}

	// Ensure contents are flushed and file is closed before reload
	_ = file.Sync()
	if err := file.Close(); err != nil {
		_ = os.Remove(serviceFilePath)
		return fmt.Errorf("close service file: %w", err)
	}

	// Reload systemd daemon
	if err := s.execSystemctl(context.TODO(), "daemon-reload"); err != nil {
		_ = os.Remove(serviceFilePath) // best-effort cleanup on failure
		return fmt.Errorf("daemon reload: %w", err)
	}

	return nil
}

// EnableService starts and enables a systemd service
func (s *SystemdService) EnableService(serviceName string) error {
	// pass args as separate elements
	if err := s.execSystemctl(context.TODO(), "enable", "--now", serviceName+".service"); err != nil {
		return fmt.Errorf("enable now: %w", err)
	}
	return nil
}

// DisableService stops and disables a systemd service
func (s *SystemdService) DisableService(serviceName string) error {
	// pass args as separate elements
	if err := s.execSystemctl(context.TODO(), "disable", "--now", serviceName+".service"); err != nil {
		return fmt.Errorf("disable now: %w", err)
	}
	return nil
}

// execSystemctl executes a systemctl command with proper error handling
func (s *SystemdService) execSystemctl(ctx context.Context, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "systemctl", args...)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	cmdStr := fmt.Sprintf("systemctl %s", strings.Join(args, " "))

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("systemd (command=%q): %w\nstdout: %s\nstderr: %s",
			cmdStr, err, stdoutBuf.String(), stderrBuf.String())
	}
	return nil
}

func templateFilePath() string {
	filePath := os.Getenv("ZMUX_REMUX_TEMPLATE_UNIT_FILE")
	if filePath == "" {
		return "templates/service.j2"
	}
	return filePath
}
