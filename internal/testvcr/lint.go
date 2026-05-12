package testvcr

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
)

// Finding describes one match of a forbidden pattern inside a cassette file.
type Finding struct {
	Path string
	Line int
	Rule string
	Hit  string
}

type lintRule struct {
	name string
	re   *regexp.Regexp
}

var lintRules = []lintRule{
	{"bearer-token", regexp.MustCompile(`Bearer [A-Za-z0-9._\-]{20,}`)},
	{"access-token-raw", regexp.MustCompile(`"AccessToken":\s*"[^R]`)},
	{"refresh-token-raw", regexp.MustCompile(`"RefreshToken":\s*"[^R]`)},
	{"pgp-private", regexp.MustCompile(`BEGIN PGP PRIVATE KEY BLOCK`)},
	{"pgp-message", regexp.MustCompile(`BEGIN PGP MESSAGE`)},
	{"proton-email", regexp.MustCompile(`@protonmail\.|@proton\.me`)},
}

// Scan walks root directories and returns findings for cassette lines matching forbidden patterns.
func Scan(roots ...string) []Finding {
	var out []Finding
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml" {
				return nil
			}
			f, err := os.Open(path)
			if err != nil {
				out = append(out, Finding{Path: path, Rule: "read-error", Hit: err.Error()})
				return nil
			}
			defer f.Close()
			s := bufio.NewScanner(f)
			s.Buffer(make([]byte, 1<<16), 1<<22)
			line := 0
			for s.Scan() {
				line++
				txt := s.Text()
				for _, rule := range lintRules {
					if m := rule.re.FindString(txt); m != "" {
						out = append(out, Finding{Path: path, Line: line, Rule: rule.name, Hit: m})
					}
				}
			}
			return nil
		})
	}
	return out
}
