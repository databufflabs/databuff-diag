package exec

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// EnvBundleCommand is one read-only probe run during environment collection.
type EnvBundleCommand struct {
	Name    string
	Command string
}

// DefaultEnvBundleCommands are the read-only probes for collect_env_bundle.
var DefaultEnvBundleCommands = []EnvBundleCommand{
	{Name: "docker-version.txt", Command: "docker version"},
	{Name: "df.txt", Command: "df -h"},
	{Name: "uname.txt", Command: "uname -a"},
	{Name: "compose-ps.txt", Command: "docker compose ps 2>/dev/null || docker-compose ps 2>/dev/null || echo 'compose not available'"},
}

// EnvBundleResult describes a generated environment bundle archive.
type EnvBundleResult struct {
	Path      string    `json:"path"`
	Filename  string    `json:"filename"`
	SizeBytes int64     `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
	Files     []string  `json:"files"`
}

// EnvBundleCollector gathers read-only host probes into a tar.gz archive.
type EnvBundleCollector struct {
	Runner   *Local
	Reports  string
	Commands []EnvBundleCommand
}

// NewEnvBundleCollector returns a collector with defaults applied.
func NewEnvBundleCollector(reportsDir string, runner *Local) *EnvBundleCollector {
	if runner == nil {
		runner = NewLocal(LocalConfig{Timeout: 60 * time.Second})
	}
	return &EnvBundleCollector{
		Runner:   runner,
		Reports:  reportsDir,
		Commands: DefaultEnvBundleCommands,
	}
}

type envBundleMeta struct {
	CollectedAt time.Time `json:"collected_at"`
	Hostname    string    `json:"hostname"`
	Files       []string  `json:"files"`
}

// Collect runs probes and writes reports/bundle-{timestamp}.tar.gz.
func (c *EnvBundleCollector) Collect(ctx context.Context) (*EnvBundleResult, error) {
	if c.Reports == "" {
		return nil, fmt.Errorf("reports directory is required")
	}
	if err := os.MkdirAll(c.Reports, 0o700); err != nil {
		return nil, fmt.Errorf("create reports dir: %w", err)
	}

	now := time.Now().UTC()
	filename := fmt.Sprintf("bundle-%s.tar.gz", now.Format("20060102-150405"))
	path := filepath.Join(c.Reports, filename)

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create bundle: %w", err)
	}
	defer file.Close()

	gw := gzip.NewWriter(file)
	tw := tar.NewWriter(gw)

	hostname, _ := os.Hostname()
	meta := envBundleMeta{
		CollectedAt: now,
		Hostname:    hostname,
	}

	for _, probe := range c.Commands {
		result, runErr := c.Runner.Run(ctx, probe.Command)
		body := formatProbeOutput(result, runErr)
		if err := writeTarEntry(tw, probe.Name, []byte(body)); err != nil {
			return nil, err
		}
		meta.Files = append(meta.Files, probe.Name)
	}

	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal meta: %w", err)
	}
	if err := writeTarEntry(tw, "meta.json", metaBytes); err != nil {
		return nil, err
	}
	meta.Files = append(meta.Files, "meta.json")

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("close tar: %w", err)
	}
	if err := gw.Close(); err != nil {
		return nil, fmt.Errorf("close gzip: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat bundle: %w", err)
	}

	return &EnvBundleResult{
		Path:      path,
		Filename:  filename,
		SizeBytes: info.Size(),
		CreatedAt: now,
		Files:     meta.Files,
	}, nil
}

func formatProbeOutput(result *Result, runErr error) string {
	if runErr != nil {
		return fmt.Sprintf("error: %v\n", runErr)
	}
	var b string
	if result.Stdout != "" {
		b += result.Stdout
		if !strings.HasSuffix(result.Stdout, "\n") {
			b += "\n"
		}
	}
	if result.Stderr != "" {
		b += "--- stderr ---\n" + result.Stderr
	}
	b += fmt.Sprintf("--- exit_code=%d duration_ms=%d timed_out=%v ---\n",
		result.ExitCode, result.DurationMS, result.TimedOut)
	return b
}

func writeTarEntry(tw *tar.Writer, name string, data []byte) error {
	hdr := &tar.Header{
		Name:    name,
		Mode:    0o644,
		Size:    int64(len(data)),
		ModTime: time.Now().UTC(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("write tar header %s: %w", name, err)
	}
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("write tar body %s: %w", name, err)
	}
	return nil
}
