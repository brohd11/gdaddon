package cmd

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
	"testing"

	"gdaddon/internal/addon"
)

// scriptedPrompter is a depPrompter reading canned answers instead of a terminal, with
// tty forced on so the prompt path runs.
func scriptedPrompter(input string) (*depPrompter, *bytes.Buffer) {
	out := &bytes.Buffer{}
	return &depPrompter{
		in:  bufio.NewReader(strings.NewReader(input)),
		out: out,
		tty: true,
	}, out
}

func depReq(repoID string) addon.DepRequest {
	return addon.DepRequest{
		Dep:        addon.Dependency{RepoID: repoID, Tag: "v1.0.0", RepoURL: "https://" + repoID},
		DeclaredBy: "root-addon",
		Action:     addon.DepAdd,
		EntryName:  "dep",
		AssetURL:   "https://" + repoID + "/releases/download/v1.0.0/dep.zip",
	}
}

func TestDepPrompterAnswers(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    bool
		wantErr error
	}{
		{"yes", "y\n", true, nil},
		{"yes spelled out", "yes\n", true, nil},
		{"no", "n\n", false, nil},
		{"empty defaults to no", "\n", false, nil},
		{"uppercase and padded", "  Y  \n", true, nil},
		{"quit aborts", "q\n", false, addon.ErrDepAborted},
		{"garbage then yes", "wat\nhuh\ny\n", true, nil},
		{"eof declines", "", false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, _ := scriptedPrompter(tt.input)
			got, err := p.confirm(depReq("example.com/o/dep"))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("confirm = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDepPrompterAllLatches checks "a" answers for the rest of the run: the second
// dependency must not consume a line of input, since there is none left.
func TestDepPrompterAllLatches(t *testing.T) {
	p, _ := scriptedPrompter("a\n")

	for i, repo := range []string{"example.com/o/one", "example.com/o/two", "example.com/o/three"} {
		ok, err := p.confirm(depReq(repo))
		if err != nil {
			t.Fatalf("dep %d: %v", i, err)
		}
		if !ok {
			t.Fatalf("dep %d was declined; 'a' should accept the rest of the run", i)
		}
	}
	if len(p.skipped) != 0 {
		t.Errorf("skipped = %+v, want none", p.skipped)
	}
}

// TestDepPrompterNonInteractive is the unattended-run rule: with no terminal to ask,
// every dependency is declined and recorded for the summary rather than installed.
func TestDepPrompterNonInteractive(t *testing.T) {
	out := &bytes.Buffer{}
	p := &depPrompter{in: bufio.NewReader(strings.NewReader("y\ny\n")), out: out, tty: false}

	for _, repo := range []string{"example.com/o/one", "example.com/o/two"} {
		ok, err := p.confirm(depReq(repo))
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			t.Errorf("%s was accepted with no tty", repo)
		}
	}
	if out.Len() != 0 {
		t.Errorf("prompted anyway: %q", out.String())
	}
	if len(p.skipped) != 2 {
		t.Fatalf("skipped %d, want 2", len(p.skipped))
	}

	summary := &bytes.Buffer{}
	p.reportSkipped(summary)
	for _, want := range []string{"example.com/o/one", "example.com/o/two", "--trust-deps", "not a terminal"} {
		if !strings.Contains(summary.String(), want) {
			t.Errorf("summary missing %q:\n%s", want, summary.String())
		}
	}
}

// TestDepPrompterBannerShowsSource checks the prompt names what the user is being asked
// to trust: who declared it, the repo, and the url that would be downloaded.
func TestDepPrompterBannerShowsSource(t *testing.T) {
	p, out := scriptedPrompter("n\n")
	req := depReq("example.com/o/dep")
	if _, err := p.confirm(req); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"root-addon", "example.com/o/dep", "@v1.0.0", req.AssetURL} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("prompt missing %q:\n%s", want, out.String())
		}
	}
}

// TestDepPrompterReportSkippedNil covers the --trust-deps path, where there is no
// prompter at all and the caller reports blindly.
func TestDepPrompterReportSkippedNil(t *testing.T) {
	var p *depPrompter
	out := &bytes.Buffer{}
	p.reportSkipped(out)
	if out.Len() != 0 {
		t.Errorf("nil prompter wrote %q", out.String())
	}
}
