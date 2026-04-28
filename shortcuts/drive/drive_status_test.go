// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

// TestDriveStatusCategorizesByHash exercises the four-bucket classification
// against a manifest piped in via --files-from and a mocked Drive listing.
func TestDriveStatusCategorizesByHash(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	// Local files referenced by the manifest:
	//   a.txt        — also on remote with different content → modified
	//   b.txt        — only local                            → new_local
	//   sub/c.txt    — also on remote with same content      → unchanged
	// Remote-only:
	//   d.txt        → new_remote
	if err := os.MkdirAll("local/sub", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile("local/a.txt", []byte("aaa"), 0o644); err != nil {
		t.Fatalf("WriteFile a.txt: %v", err)
	}
	if err := os.WriteFile("local/b.txt", []byte("bbb"), 0o644); err != nil {
		t.Fatalf("WriteFile b.txt: %v", err)
	}
	if err := os.WriteFile("local/sub/c.txt", []byte("ccc"), 0o644); err != nil {
		t.Fatalf("WriteFile sub/c.txt: %v", err)
	}

	// Manifest mixes the three accepted formats: "rel" / "./root/rel" / "root/rel".
	manifest := strings.Join([]string{
		"a.txt",
		"./local/b.txt",
		"local/sub/c.txt",
	}, "\n")

	// Root folder list — order matters: stubs match in registration order.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_a", "name": "a.txt", "type": "file"},
					map[string]interface{}{"token": "tok_sub", "name": "sub", "type": "folder"},
					map[string]interface{}{"token": "tok_d", "name": "d.txt", "type": "file"},
					// noise: an online doc and a shortcut should be ignored
					map[string]interface{}{"token": "tok_doc", "name": "ignored.docx", "type": "docx"},
					map[string]interface{}{"token": "tok_sc", "name": "ignored.lnk", "type": "shortcut"},
				},
				"has_more": false,
			},
		},
	})

	// Subfolder list
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=tok_sub",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_c", "name": "c.txt", "type": "file"},
				},
				"has_more": false,
			},
		},
	})

	// Download a.txt: remote content differs from local "aaa" → modified.
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/tok_a/download",
		Status:  200,
		Body:    []byte("AAA"),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})

	// Download c.txt: remote content matches local "ccc" → unchanged.
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/tok_c/download",
		Status:  200,
		Body:    []byte("ccc"),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})

	err := mountAndRunDrive(t, DriveStatus, []string{
		"+status",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--files-from", manifest,
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	out := stdout.String()
	checks := []struct {
		bucket string
		path   string
		token  string
	}{
		{"new_local", "b.txt", ""},
		{"new_remote", "d.txt", "tok_d"},
		{"modified", "a.txt", "tok_a"},
		{"unchanged", "sub/c.txt", "tok_c"},
	}
	for _, c := range checks {
		if !strings.Contains(out, `"`+c.bucket+`":`) {
			t.Errorf("output missing bucket %q\noutput: %s", c.bucket, out)
		}
		if !strings.Contains(out, `"rel_path": "`+c.path+`"`) {
			t.Errorf("output missing rel_path %q (expected in %s)\noutput: %s", c.path, c.bucket, out)
		}
		if c.token != "" && !strings.Contains(out, `"file_token": "`+c.token+`"`) {
			t.Errorf("output missing file_token %q (expected in %s)\noutput: %s", c.token, c.bucket, out)
		}
	}

	if strings.Contains(out, "ignored.docx") || strings.Contains(out, "ignored.lnk") {
		t.Errorf("output should skip docx/shortcut entries\noutput: %s", out)
	}

	reg.Verify(t)
}

func TestDriveStatusRejectsMissingLocalDir(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	err := mountAndRunDrive(t, DriveStatus, []string{
		"+status",
		"--local-dir", "does-not-exist",
		"--folder-token", "folder_root",
		"--files-from", "a.txt",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for missing local dir, got nil")
	}
}

func TestDriveStatusRejectsLocalFile(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.WriteFile("not-a-dir.txt", []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := mountAndRunDrive(t, DriveStatus, []string{
		"+status",
		"--local-dir", "not-a-dir.txt",
		"--folder-token", "folder_root",
		"--files-from", "a.txt",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error when --local-dir is a file, got nil")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestDriveStatusRejectsEmptyManifest(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	err := mountAndRunDrive(t, DriveStatus, []string{
		"+status",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--files-from", "   \n  \n",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for empty manifest, got nil")
	}
}

func TestNormalizeManifestPath(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		root string
		want string
	}{
		{"already-relative", "a/b.txt", "local", "a/b.txt"},
		{"prefixed-with-root", "local/a/b.txt", "local", "a/b.txt"},
		{"find-style-dot-slash", "./local/a/b.txt", "local", "a/b.txt"},
		{"root-is-cwd-dot", "./a/b.txt", ".", "a/b.txt"},
		{"escaping-root-rejected", "../escape", "local", ""},
		{"root-itself-rejected", "local", "local", ""},
		{"empty-after-clean", ".", "local", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := normalizeManifestPath(c.raw, c.root)
			if got != c.want {
				t.Errorf("normalizeManifestPath(%q, %q) = %q, want %q", c.raw, c.root, got, c.want)
			}
		})
	}
}
