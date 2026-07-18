package buildidentity

import (
	"archive/tar"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportCommittedGoTreeExcludesIgnoredWorktreeFiles(t *testing.T) {
	repository := t.TempDir()
	goRoot := filepath.Join(repository, "src", "go")
	if err := os.MkdirAll(goRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		".gitignore":        "/src/go/ignored.go\n",
		"src/go/go.mod":     "module example.test/committed\n\ngo 1.26\n",
		"src/go/tracked.go": "package committed\n",
		"src/go/ignored.go": "package ignored\n",
	}
	for name, content := range files {
		path := filepath.Join(repository, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	commands := [][]string{
		{"init"},
		{"add", ".gitignore", "src/go/go.mod", "src/go/tracked.go"},
		{
			"-c", "user.name=jobmgr-test",
			"-c", "user.email=jobmgr-test@example.invalid",
			"commit", "-m", "fixture",
		},
	}
	for _, arguments := range commands {
		command := exec.Command("git", arguments...)
		command.Dir = repository
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf(
				"git %s: %v: %s",
				strings.Join(arguments, " "),
				err,
				output,
			)
		}
	}
	destination := filepath.Join(t.TempDir(), "export")
	if err := ExportCommittedGoTree(
		context.Background(),
		goRoot,
		destination,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destination, "tracked.go")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destination, "ignored.go")); !os.IsNotExist(err) {
		t.Fatalf("ignored source escaped into committed export: %v", err)
	}
}

func TestCommittedTreeExtractionRejectsUnsafeEntries(t *testing.T) {
	tests := map[string]tar.Header{
		"parent traversal": {
			Name: "../escape",
			Mode: 0o600,
			Size: 1,
		},
		"absolute path": {
			Name: "/escape",
			Mode: 0o600,
			Size: 1,
		},
		"escaping symlink": {
			Name:     "safe/link",
			Typeflag: tar.TypeSymlink,
			Linkname: "../../escape",
		},
		"hard link": {
			Name:     "safe/link",
			Typeflag: tar.TypeLink,
			Linkname: "safe/target",
		},
	}
	for name, header := range tests {
		t.Run(name, func(t *testing.T) {
			var archive bytes.Buffer
			writer := tar.NewWriter(&archive)
			if header.Typeflag == 0 {
				header.Typeflag = tar.TypeReg
			}
			if err := writer.WriteHeader(&header); err != nil {
				t.Fatal(err)
			}
			if header.Size != 0 {
				if _, err := writer.Write([]byte("x")); err != nil {
					t.Fatal(err)
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if err := extractCommittedTree(
				t.TempDir(),
				bytes.NewReader(archive.Bytes()),
			); err == nil {
				t.Fatal("unsafe archive entry was accepted")
			}
		})
	}
}
