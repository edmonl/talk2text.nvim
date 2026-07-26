package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestParseTranscriptPathPreservesRuntimePath(t *testing.T) {
	root := t.TempDir()
	runtimeDir := filepath.Join(root, "runtime with spaces")
	resolvedTranscriptDir := filepath.Join(root, "recordings")
	if err := os.Mkdir(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(resolvedTranscriptDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(resolvedTranscriptDir, filepath.Join(runtimeDir, "transcripts")); err != nil {
		t.Fatal(err)
	}

	transcript := filepath.Join(runtimeDir, "transcripts", "42")
	gotRuntime, gotID, err := parseTranscriptPath(transcript)
	if err != nil {
		t.Fatalf("parseTranscriptPath() error = %v", err)
	}
	if gotRuntime != runtimeDir {
		t.Fatalf("parseTranscriptPath() runtime directory = %q, want symlink path %q", gotRuntime, runtimeDir)
	}
	if gotID != 42 {
		t.Fatalf("parseTranscriptPath() transcript ID = %d, want 42", gotID)
	}
}

func TestParseTranscriptPathRejectsRoot(t *testing.T) {
	if _, _, err := parseTranscriptPath("/transcripts/1"); err == nil {
		t.Fatal("parseTranscriptPath() accepted the filesystem root")
	}
}

func TestParseTranscriptPathRejectsInvalidPath(t *testing.T) {
	for _, path := range []string{"runtime/transcripts/1", "/runtime/transcripts/"} {
		if _, _, err := parseTranscriptPath(path); err == nil || err.Error() != "transcript path must be an absolute file path" {
			t.Errorf("parseTranscriptPath(%q) error = %v, want absolute-path error", path, err)
		}
	}
}

func TestParseTranscriptPathRejectsMalformedNames(t *testing.T) {
	runtimeDir := t.TempDir()
	for _, filename := range []string{
		"0",
		"01",
		"1.txt",
		strconv.FormatUint(uint64(^uint(0)>>1)+1, 10),
	} {
		path := filepath.Join(runtimeDir, "transcripts", filename)
		if _, _, err := parseTranscriptPath(path); err == nil {
			t.Errorf("parseTranscriptPath(%q) accepted a malformed filename", path)
		}
	}
}
