// Go port of changelog.js: builds a markdown changelog from the git log between two refs.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

const emptyComponent = "$$"

var (
	reCloses      = regexp.MustCompile(`(?:closes|fixes|features)\s#?([a-z0-9_\-]+)`)
	reBreaking    = regexp.MustCompile(`BREAKING CHANGE:([\s\S]*)`)
	reConvention  = regexp.MustCompile(`^(.*)\((.*)\)\s*:\s*(.*)$`)
	reBracket     = regexp.MustCompile(`^\[([^ \]]*)\]\s*\[([^\]]*)\]\s*(.*)$`)
	reIssue       = regexp.MustCompile(`^[A-Z]+-[0-9]+$`)
	reVersionSufx = regexp.MustCompile(`-.*$`)
	featTypes     = []string{"feat", "pods", "pod", "config", "conf", "refactor", "codereview"}
)

type commit struct {
	hash, subject, body, typ, component, breaking string
	closes                                        []string
}

type generator struct {
	dir  string
	lite bool
	out  *strings.Builder
}

func gitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	return string(out), err
}

// envJS returns an environment variable as the JS `process.env.NAME` string interpolation did:
// an unset variable becomes the literal "undefined".
func envJS(name string) string {
	if v, ok := os.LookupEnv(name); ok {
		return v
	}
	return "undefined"
}

func authorOf(body string) string {
	lines := strings.Split(body, "\n")
	return lines[len(lines)-1]
}

func parseRawCommit(raw string) *commit {
	if raw == "" {
		return nil
	}
	lines := strings.Split(raw, "\n")
	c := &commit{hash: lines[0]}
	lines = lines[1:]
	if len(lines) > 0 {
		c.subject = lines[0]
		lines = lines[1:]
	}
	for _, line := range lines {
		if m := reCloses.FindStringSubmatch(strings.ToLower(line)); m != nil {
			c.closes = append(c.closes, strings.ToUpper(m[1]))
		}
	}
	if m := reBreaking.FindStringSubmatch(raw); m != nil {
		fmt.Fprintf(os.Stderr, "BREAK >%s<\n", m[1])
		c.breaking = m[1]
	}
	c.body = strings.Join(lines, "\n")
	if strings.Contains(c.subject, "Merge ") {
		return nil
	}
	m := reConvention.FindStringSubmatch(c.subject)
	if m == nil || m[1] == "" || m[3] == "" {
		m = reBracket.FindStringSubmatch(c.subject)
	}
	if m == nil || m[1] == "" || m[3] == "" {
		fmt.Fprintf(os.Stderr, "WARNING: Incorrect message: %s %s\n", c.hash, c.subject)
		c.typ = "bof"
		c.component = "?"
		c.subject = c.subject + " par *" + authorOf(c.body) + "*"
		return c
	}
	c.typ = strings.ToLower(m[1])
	prefix := ""
	for i, t := range featTypes {
		if i > 0 && t == c.typ { // same quirk as the JS: indexOf(type) > 0
			prefix = "[" + c.typ + "] - "
			c.typ = "feat"
			break
		}
	}
	c.component = m[2]
	c.subject = prefix + m[3]
	return c
}

func (g *generator) linkToIssue(issue string) string {
	if reIssue.MatchString(issue) {
		if g.lite {
			return fmt.Sprintf("[#%s]", issue)
		}
		return fmt.Sprintf("[#%s](%s/%s)", issue, envJS("ISSUE_TRACKER"), issue)
	}
	if g.lite {
		return fmt.Sprintf("[%s]", issue)
	}
	return fmt.Sprintf("[%s](https://wiki.services.local/dosearchsite.action?spaceSearch=false&queryString='%s')", issue, issue)
}

func (g *generator) linkToCommit(hash string) string {
	if g.lite {
		return ""
	}
	short := hash
	if len(short) > 8 {
		short = short[:8]
	}
	return fmt.Sprintf("([%s](%s/%s))", short, envJS("GIT_COMMIT_LINK"), hash)
}

// currentDate formats a `git log --format=%ai` date as YYYY-MM-DD in local time (like the JS Date did).
func currentDate(date string) string {
	now := time.Now()
	if date = strings.TrimSpace(date); date != "" {
		if t, err := time.Parse("2006-01-02 15:04:05 -0700", date); err == nil {
			now = t.Local()
		}
	}
	return now.Format("2006-01-02")
}

func (g *generator) linksFor(closes []string) string {
	links := make([]string, len(closes))
	for i, c := range closes {
		links[i] = g.linkToIssue(c)
	}
	return ",\n   " + strings.Join(links, ", ")
}

func (g *generator) printSection(title string, section map[string][]*commit, printCommitLinks bool) {
	components := make([]string, 0, len(section))
	for name := range section {
		components = append(components, name)
	}
	sort.Strings(components)
	if len(components) == 0 || components[0] == emptyComponent {
		return
	}
	fmt.Fprintf(g.out, "\n\n## %s", title)
	for _, name := range components {
		prefix := "\n  -"
		nested := len(section[name]) > 1
		if name != emptyComponent {
			if nested {
				fmt.Fprintf(g.out, "\n  - **%s:**\n", name)
				prefix = "\n    -"
			} else {
				prefix = fmt.Sprintf("\n  - **%s:**", name)
			}
		}
		doublon := ""
		for _, c := range section[name] {
			if printCommitLinks {
				if doublon == c.subject {
					g.out.WriteString(g.linkToCommit(c.hash))
				} else {
					fmt.Fprintf(g.out, "%s %s\n  %s", prefix, c.subject, g.linkToCommit(c.hash))
				}
			} else if doublon != c.subject {
				fmt.Fprintf(g.out, "%s %s\n", prefix, c.subject)
			}
			if len(c.closes) > 0 {
				g.out.WriteString(g.linksFor(c.closes))
			}
			doublon = c.subject
		}
	}
}

func (g *generator) readGitLog(from, to string) ([]*commit, error) {
	raw, err := gitOut(g.dir, "log", "--invert-grep", "--grep=^Merge", "-E", "--format=%H%n%B%n%an%n==END==", from+".."+to)
	if err != nil {
		return nil, err
	}
	var commits []*commit
	for _, chunk := range strings.Split(raw, "\n==END==\n") {
		if c := parseRawCommit(chunk); c != nil {
			commits = append(commits, c)
		}
	}
	return commits, nil
}

func (g *generator) writeChangelog(data string, commits []*commit, version, date string) {
	sections := map[string]map[string][]*commit{
		"fix": {}, "feat": {}, "perf": {}, "bof": {}, "breaks": {emptyComponent: {}},
	}
	for _, c := range commits {
		component := c.component
		if component == "" {
			component = emptyComponent
		}
		if section, ok := sections[c.typ]; ok {
			section[component] = append(section[component], c)
		}
		if c.breaking != "" {
			sections["breaks"][component] = append(sections["breaks"][component], &commit{
				subject: fmt.Sprintf("due to %s,\n %s", g.linkToCommit(c.hash), c.breaking),
				hash:    c.hash,
			})
		}
	}
	if version == "HEAD" {
		g.out.WriteString(" ") // util.format("", "") in the JS produced a single space
	} else {
		fmt.Fprintf(g.out, "\n<a name='%s'></a>\n# %s (%s)\n\n", reVersionSufx.ReplaceAllString(version, ""), version, currentDate(date))
	}
	g.printSection("Bug Fixes", sections["fix"], true)
	g.printSection("Features", sections["feat"], true)
	g.printSection("Performance Improvements", sections["perf"], true)
	g.printSection("Breaking Changes", sections["breaks"], false)
	g.printSection("non conforme", sections["bof"], true)
	g.out.WriteString("\n\n" + data)
}

// generateChangelog mirrors `changelog.js <to> <file> <from> [--lite]` run in dir.
// With a file, the existing content is kept below the new section and the file is rewritten;
// otherwise the markdown is returned. An empty from falls back to the latest tag.
func generateChangelog(dir, to, file, from string, lite bool) (string, error) {
	g := &generator{dir: dir, lite: lite, out: &strings.Builder{}}
	data := ""
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		data = string(b)
	}
	tagDate, err := gitOut(dir, "log", "-1", "--format=%ai", to)
	if err != nil {
		return "", fmt.Errorf("cannot get date of %s: %w", to, err)
	}
	if from == "" {
		if from, err = gitOut(dir, "describe", "--tags", "--abbrev=0"); err != nil {
			return "", fmt.Errorf("cannot get the previous tag: %w", err)
		}
		from = strings.TrimSpace(from)
		to = "HEAD"
	}
	fmt.Fprintf(os.Stderr, "Reading git log between %s and %s (%s)\n", from, to, strings.TrimSpace(tagDate))
	commits, err := g.readGitLog(from, to)
	if err != nil {
		return "", err
	}
	if to == "HEAD" {
		// changelog.js crashed here (commits[0] undefined) and produced nothing: keep the file
		// untouched and return an empty changelog, but say so.
		if len(commits) == 0 {
			fmt.Fprintf(os.Stderr, "WARNING: no commits between %s and HEAD, empty changelog\n", from)
			return "", nil
		}
		commits[0].subject += " *Last*"
		tagDate = ""
	}
	fmt.Fprintf(os.Stderr, "Parsed %d commits\n", len(commits))
	g.writeChangelog(data, commits, to, tagDate)
	if file != "" {
		return "", os.WriteFile(file, []byte(g.out.String()), 0o644)
	}
	return g.out.String(), nil
}
