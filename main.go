// Bitrise step "generate Changelog": Go port of the former step.sh.
// Markdown generation lives in changelog.go, HTML rendering in html.go.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}

// run executes a command in dir, streaming its output, and exits on failure (like `set -e`).
func run(dir string, env []string, name string, args ...string) {
	fmt.Fprintln(os.Stderr, "+", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fail("%s failed: %v", name, err)
	}
}

// capture runs a command and returns its trimmed stdout (like `$(...)`); ignoreErr mimics `|| true`.
func capture(dir string, ignoreErr bool, name string, args ...string) string {
	fmt.Fprintln(os.Stderr, "+", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil && !ignoreErr {
		fail("%s failed: %v", name, err)
	}
	return strings.TrimRight(string(out), "\n")
}

func main() {
	stepDir := os.Getenv("BITRISE_STEP_SOURCE_DIR")
	if stepDir == "" {
		var err error
		if stepDir, err = os.Getwd(); err != nil {
			fail("cannot get working dir: %v", err)
		}
	}
	srcDir := os.Getenv("BITRISE_SOURCE_DIR")
	if srcDir == "" {
		fail("BITRISE_SOURCE_DIR is not set")
	}
	tagDest := os.Getenv("TAG_DEST")
	changeFile := os.Getenv("CHANGE_FILE")
	branch := os.Getenv("BITRISE_GIT_BRANCH")

	var previousTag string
	if tagDest == "HEAD" {
		// as in branch test_origin: the changelog of HEAD always starts at its first parent
		tagHead := capture(srcDir, false, "git", "tag", "--points-at", "HEAD")
		first := strings.SplitN(capture(srcDir, false, "git", "rev-list", "--parents", "HEAD"), "\n", 2)[0]
		fields := strings.Fields(first)
		if len(fields) < 2 {
			fail("HEAD has no parent")
		}
		previousTag = fields[1]
		if tagHead != "" {
			tagDest = tagHead
		}
	}

	if tagDest != "HEAD" {
		run(srcDir, nil, "git", "-c", "core.hooksPath=/dev/null", "checkout", tagDest+"~1")
		previousTag = capture(srcDir, false, "git", "describe", "--tags", "--abbrev=0")
	}

	run(srcDir, nil, "git", "-c", "core.hooksPath=/dev/null", "checkout", tagDest)

	if changeFile != "" {
		if capture(srcDir, true, "git", "config", "user.name") == "" {
			run(srcDir, nil, "git", "config", "user.email", os.Getenv("IC_COMMITER_MAIL"))
			run(srcDir, nil, "git", "config", "user.name", os.Getenv("IC_COMMITER_NAME"))
		}

		changePath := changeFile
		if !filepath.IsAbs(changePath) {
			changePath = filepath.Join(srcDir, changeFile)
		}
		f, err := os.OpenFile(changePath, os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			fail("cannot touch %s: %v", changePath, err)
		}
		f.Close()

		content, err := os.ReadFile(changePath)
		if err != nil {
			fail("cannot read %s: %v", changePath, err)
		}
		already := false
		for _, line := range strings.Split(string(content), "\n") {
			if strings.HasPrefix(line, "# "+tagDest) {
				already = true
				break
			}
		}

		if !already {
			if _, err := generateChangelog(srcDir, tagDest, changePath, previousTag, false); err != nil {
				fail("changelog generation failed: %v", err)
			}
			if os.Getenv("CI") == "true" {
				gitEnv := []string{"GIT_ASKPASS=echo", "GIT_SSH=" + filepath.Join(stepDir, "ssh_no_prompt.sh")}
				run(srcDir, gitEnv, "git", "add", changeFile)
				run(srcDir, gitEnv, "git", "commit", "-m", fmt.Sprintf("chore(%s):update changes [skip ci]", tagDest))
				run(srcDir, gitEnv, "git", "push", "origin", "HEAD:"+branch)
			}
		}
	}

	changelog, err := generateChangelog(srcDir, tagDest, "", previousTag, true)
	if err != nil {
		fail("changelog generation failed: %v", err)
	}
	changelog = strings.TrimRight(changelog, "\n") // like $(...) in the shell

	if changelog != "" { // as in branch test_origin: no changelog.html for an empty changelog
		// to_html.rb: `puts markdown.render(md)` adds a newline only when the output lacks one
		html := markdownToHTML(changelog)
		if !strings.HasSuffix(html, "\n") {
			html += "\n"
		}
		if err := os.WriteFile(filepath.Join(srcDir, "changelog.html"), []byte(html), 0o644); err != nil {
			fail("cannot write changelog.html: %v", err)
		}
	}

	run(srcDir, nil, "envman", "add", "--key", "CHANGELOG", "--value", changelog)
}
