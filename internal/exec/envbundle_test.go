package exec

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestEnvBundleCollector_Collect(t *testing.T) {
	dir := t.TempDir()
	collector := NewEnvBundleCollector(dir, NewLocal(LocalConfig{Timeout: 30 * time.Second}))

	result, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if result.Filename == "" {
		t.Fatal("filename is empty")
	}
	if result.SizeBytes <= 0 {
		t.Fatalf("size_bytes = %d, want > 0", result.SizeBytes)
	}

	info, err := os.Stat(result.Path)
	if err != nil {
		t.Fatalf("stat bundle: %v", err)
	}
	if info.Size() != result.SizeBytes {
		t.Fatalf("stat size = %d, result size = %d", info.Size(), result.SizeBytes)
	}

	f, err := os.Open(result.Path)
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	names := map[string]bool{}
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		names[hdr.Name] = true
	}

	for _, want := range []string{"docker-version.txt", "df.txt", "uname.txt", "compose-ps.txt", "meta.json"} {
		if !names[want] {
			t.Fatalf("missing tar entry %q, got %v", want, names)
		}
	}
}

func TestEnvBundleCollector_EmptyReportsDir(t *testing.T) {
	collector := NewEnvBundleCollector("", nil)
	_, err := collector.Collect(context.Background())
	if err == nil {
		t.Fatal("expected error for empty reports dir")
	}
}

func TestFormatProbeOutput(t *testing.T) {
	out := formatProbeOutput(&Result{Stdout: "ok", ExitCode: 0}, nil)
	if !strings.Contains(out, "ok") {
		t.Fatalf("output = %q, want ok", out)
	}
	if !strings.Contains(out, "exit_code=0") {
		t.Fatalf("output = %q, want exit_code", out)
	}
}
