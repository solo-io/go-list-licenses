package license

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/solo-io/go-list-licenses/assets"
)

type Template struct {
	Title    string
	Nickname string
	Words    map[string]int
}

func parseTemplate(content string) (*Template, error) {
	t := Template{}
	text := []byte{}
	state := 0
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if state == 0 {
			if line == "---" {
				state = 1
			}
		} else if state == 1 {
			if line == "---" {
				state = 2
			} else {
				if strings.HasPrefix(line, "title:") {
					t.Title = strings.TrimSpace(line[len("title:"):])
				} else if strings.HasPrefix(line, "nickname:") {
					t.Nickname = strings.TrimSpace(line[len("nickname:"):])
				}
			}
		} else if state == 2 {
			text = append(text, scanner.Bytes()...)
			text = append(text, []byte("\n")...)
		}
	}
	t.Words = makeWordSet(text)
	return &t, scanner.Err()
}

func GetTemplatesSet() (map[string]interface{}, error) {
	templates, err := loadTemplates()
	if err != nil {
		return nil, err
	}
	ret := map[string]interface{}{}
	for _, t := range templates {
		ret[t.Title] = true
	}
	return ret, nil
}

func loadTemplates() ([]*Template, error) {
	templates := []*Template{}
	for _, a := range assets.Assets {
		templ, err := parseTemplate(a.Content)
		if err != nil {
			return nil, err
		}
		templates = append(templates, templ)
	}
	return templates, nil
}

var (
	reWords     = regexp.MustCompile(`[\w']+`)
	reCopyright = regexp.MustCompile(
		`(?i)\s*Copyright (?:©|\(c\)|\xC2\xA9)?\s*(?:\d{4}|\[year\]).*`)
)

func cleanLicenseData(data []byte) []byte {
	data = bytes.ToLower(data)
	data = reCopyright.ReplaceAll(data, nil)
	return data
}

func makeWordSet(data []byte) map[string]int {
	words := map[string]int{}
	data = cleanLicenseData(data)
	matches := reWords.FindAll(data, -1)
	for i, m := range matches {
		s := string(m)
		if _, ok := words[s]; !ok {
			// Non-matching words are likely in the license header, to mention
			// copyrights and authors. Try to preserve the initial sequences,
			// to display them later.
			words[s] = i
		}
	}
	return words
}

type Word struct {
	Text string
	Pos  int
}

type sortedWords []Word

func (s sortedWords) Len() int {
	return len(s)
}

func (s sortedWords) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s sortedWords) Less(i, j int) bool {
	return s[i].Pos < s[j].Pos
}

type MatchResult struct {
	Template     *Template
	Score        float64
	ExtraWords   []string
	MissingWords []string
	FileContent  []byte
}

func sortAndReturnWords(words []Word) []string {
	sort.Sort(sortedWords(words))
	tokens := []string{}
	for _, w := range words {
		tokens = append(tokens, w.Text)
	}
	return tokens
}

// matchTemplates returns the best license template matching supplied data,
// its score between 0 and 1 and the list of words appearing in license but not
// in the matched template.
func matchTemplates(license []byte, templates []*Template) MatchResult {
	bestScore := float64(-1)
	var bestTemplate *Template
	bestExtra := []Word{}
	bestMissing := []Word{}
	words := makeWordSet(license)
	for _, t := range templates {
		extra := []Word{}
		missing := []Word{}
		common := 0
		for w, pos := range words {
			_, ok := t.Words[w]
			if ok {
				common++
			} else {
				extra = append(extra, Word{
					Text: w,
					Pos:  pos,
				})
			}
		}
		for w, pos := range t.Words {
			if _, ok := words[w]; !ok {
				missing = append(missing, Word{
					Text: w,
					Pos:  pos,
				})
			}
		}
		score := 2 * float64(common) / (float64(len(words)) + float64(len(t.Words)))
		if score > bestScore {
			bestScore = score
			bestTemplate = t
			bestMissing = missing
			bestExtra = extra
		}
	}
	return MatchResult{
		Template:     bestTemplate,
		Score:        bestScore,
		ExtraWords:   sortAndReturnWords(bestExtra),
		MissingWords: sortAndReturnWords(bestMissing),
		FileContent:  license,
	}
}

type MissingError struct {
	Err string
}

func (err *MissingError) Error() string {
	return err.Err
}

func getPackagesInfo(gopath string, pkgs []string) ([]*PkgInfo, error) {
	args := []string{"list", "-e", "-json"}
	// TODO: split the list for platforms which do not support massive argument lists.
	args = append(args, pkgs...)
	cmd := exec.Command("go", args...)
	cmd.Env = fixEnv(gopath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("go %s failed with:\n%s",
			strings.Join(args, " "), string(out))
	}
	infos := make([]*PkgInfo, 0, len(pkgs))
	decoder := json.NewDecoder(bytes.NewBuffer(out))
	for _, pkg := range pkgs {
		info := &PkgInfo{}
		err := decoder.Decode(info)
		if err != nil {
			return nil, fmt.Errorf("could not retrieve package information for %s", pkg)
		}
		if pkg != info.ImportPath {
			return nil, fmt.Errorf("package information mismatch: asked for %s, got %s",
				pkg, info.ImportPath)
		}
		if info.Error != nil && info.Name == "" {
			info.Name = info.ImportPath
		}
		infos = append(infos, info)
	}
	return infos, err
}

var (
	reLicense = regexp.MustCompile(`(?i)^(?:` +
		`((?:un)?licen[sc]e)|` +
		`((?:un)?licen[sc]e\.(?:md|markdown|txt))|` +
		`(copy(?:ing|right)(?:\.[^.]+)?)|` +
		`(licen[sc]e\.[^.]+)` +
		`)$`)
)

// scoreLicenseName returns a factor between 0 and 1 weighting how likely
// supplied filename is a license file.
func scoreLicenseName(name string) float64 {
	m := reLicense.FindStringSubmatch(name)
	switch {
	case m == nil:
		break
	case m[1] != "":
		return 1.0
	case m[2] != "":
		return 0.9
	case m[3] != "":
		return 0.8
	case m[4] != "":
		return 0.7
	}
	return 0.
}

// findLicense looks for license files in package import path, and down to
// parent directories until a file is found or $GOPATH/src is reached. It
// returns the path and score of the best entry, an empty string if none was
// found.
func findLicense(info *PkgInfo) (string, error) {
	lookPath := info.Root
	fis, err := ioutil.ReadDir(lookPath)
	if err != nil {
		println(fmt.Sprintf("%+v\n", info))
		return "", errors.Wrapf(err, "unable to read dir at %s", lookPath)
	}
	bestScore := float64(0)
	bestName := ""
	for _, fi := range fis {
		if !fi.Mode().IsRegular() {
			continue
		}
		score := scoreLicenseName(fi.Name())
		if score > bestScore {
			bestScore = score
			bestName = fi.Name()
		}
	}
	if bestName != "" {
		return filepath.Join(lookPath, bestName), nil
	}
	return "", nil
}

type License struct {
	Package  string
	Score    float64
	Template *Template
	Path     string
	// ManualPath indicates the URI of the license file. If provided, overrides the auto-generated URI path
	ManualPath   string
	Err          string
	ExtraWords   []string
	MissingWords []string
	// text from the license file
	FileContent []byte
}

func listLicenses(pkgs []string, includeIndirectDeps bool) ([]License, error) {
	templates, err := loadTemplates()
	if err != nil {
		return nil, err
	}
	var infos []*PkgInfo
	stdSet := map[string]bool{}

	infos, err = listModDependencies(includeIndirectDeps)
	if err != nil {
		if _, ok := err.(*MissingError); ok {
			return nil, err
		}
		return nil, fmt.Errorf("could not list %s dependencies: %s",
			strings.Join(pkgs, " "), err)
	}

	// Cache matched licenses by path. Useful for package with a lot of
	// subpackages like bleve.
	matched := map[string]MatchResult{}

	licenses := []License{}
	for _, info := range infos {
		if info.Error != nil {
			licenses = append(licenses, License{
				Package: info.Name,
				Err:     info.Error.Err,
			})
			continue
		}
		if stdSet[info.ImportPath] {
			continue
		}
		path, err := findLicense(info)
		if err != nil {
			return nil, err
		}
		if strings.Contains(info.ImportPath, "solo-io") {
			continue
		}
		license := License{
			Package: info.ImportPath,
			Path:    path,
		}
		if path != "" {
			fpath := filepath.Join(path)
			m, ok := matched[fpath]
			if !ok {
				data, err := ioutil.ReadFile(fpath)
				if err != nil {
					return nil, errors.Wrapf(err, "Unable to read file at %s", fpath)
				}
				m = matchTemplates(data, templates)
				matched[fpath] = m
			}
			license.Score = m.Score
			license.Template = m.Template
			license.ExtraWords = m.ExtraWords
			license.MissingWords = m.MissingWords
			license.FileContent = m.FileContent
		}
		licenses = append(licenses, license)
	}
	return licenses, nil
}

// longestCommonPrefix returns the longest common prefix over import path
// components of supplied licenses.
func longestCommonPrefix(licenses []License) string {
	type Node struct {
		Name     string
		Children map[string]*Node
	}
	// Build a prefix tree. Not super efficient, but easy to do.
	root := &Node{
		Children: map[string]*Node{},
	}
	for _, l := range licenses {
		n := root
		for _, part := range strings.Split(l.Package, "/") {
			c := n.Children[part]
			if c == nil {
				c = &Node{
					Name:     part,
					Children: map[string]*Node{},
				}
				n.Children[part] = c
			}
			n = c
		}
	}
	n := root
	prefix := []string{}
	for {
		if len(n.Children) != 1 {
			break
		}
		for _, c := range n.Children {
			prefix = append(prefix, c.Name)
			n = c
			break
		}
	}
	return strings.Join(prefix, "/")
}

// groupLicenses returns the input licenses after grouping them by license path
// and find their longest import path common prefix. Entries with empty paths
// are left unchanged.
func groupLicenses(licenses []License) ([]License, error) {
	paths := map[string][]License{}
	for _, l := range licenses {
		if l.Path == "" {
			continue
		}
		paths[l.Path] = append(paths[l.Path], l)
	}
	for k, v := range paths {
		if len(v) <= 1 {
			continue
		}
		prefix := longestCommonPrefix(v)
		if prefix == "" {
			return nil, fmt.Errorf(
				"packages share the same license but not common prefix: %v", v)
		}
		l := v[0]
		l.Package = prefix
		paths[k] = []License{l}
	}
	kept := []License{}
	for _, l := range licenses {
		if l.Path == "" {
			kept = append(kept, l)
			continue
		}
		if v, ok := paths[l.Path]; ok {
			kept = append(kept, v[0])
			delete(paths, l.Path)
		}
	}
	return kept, nil
}
