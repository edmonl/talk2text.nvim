package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunUsesOutputKindEnvironment(t *testing.T) {
	runtimeDir := t.TempDir()
	transcriptDir := filepath.Join(runtimeDir, "transcripts")
	if err := os.Mkdir(transcriptDir, 0o700); err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join(transcriptDir, "1")
	if err := os.WriteFile(transcript, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(outputKindEnv, "blank")
	t.Setenv("TALK2TEXT_NOTIFY_CMD", "")

	if err := run([]string{transcript}); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if _, err := os.Lstat(transcript); !os.IsNotExist(err) {
		t.Fatalf("blank transcript was not removed: %v", err)
	}
}

func TestRunRejectsInvalidCommandContract(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind string
		args []string
		want string
	}{
		{
			name: "missing path",
			kind: "text",
			want: "usage:",
		},
		{
			name: "old argument shape",
			args: []string{"text", "/runtime/transcripts/1"},
			want: "usage:",
		},
		{
			name: "missing kind",
			args: []string{"/runtime/transcripts/1"},
			want: "unknown transcript kind",
		},
		{
			name: "unknown kind",
			kind: "other",
			args: []string{"/runtime/transcripts/1"},
			want: "unknown transcript kind",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(outputKindEnv, tc.kind)
			err := run(tc.args)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("run() error = %v, want text containing %q", err, tc.want)
			}
		})
	}
}
