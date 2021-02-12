package license

import (
	"encoding/csv"
	"flag"
	"fmt"
	"github.com/solo-io/go-list-licenses/pkg/markdown"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
)

// fixEnv returns a copy of the process environment where GOPATH is adjusted to
// supplied value. It returns nil if gopath is empty.
func fixEnv(gopath string) []string {
	if gopath == "" {
		return nil
	}
	kept := []string{
		"GOPATH=" + gopath,
	}
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "GOPATH=") {
			kept = append(kept, env)
		}
	}
	return kept
}

// expandPackages takes a list of package or package expressions and invoke go
// list to expand them to packages. In particular, it handles things like "..."
// and ".".
func expandPackages(gopath string, pkgs []string) ([]string, error) {
	args := []string{"list"}
	args = append(args, pkgs...)
	cmd := exec.Command("go", args...)
	cmd.Env = fixEnv(gopath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		output := string(out)
		if strings.Contains(output, "cannot find package") ||
			strings.Contains(output, "no buildable Go source files") {
			return nil, &MissingError{Err: output}
		}
		return nil, fmt.Errorf("'go %s' failed with:\n%s",
			strings.Join(args, " "), output)
	}
	names := []string{}
	for _, s := range strings.Split(string(out), "\n") {
		s = strings.TrimSpace(s)
		if s != "" {
			names = append(names, s)
		}
	}
	return names, nil
}

func listPackagesAndDeps(gopath string, pkgs []string) ([]string, error) {
	pkgs, err := expandPackages(gopath, pkgs)
	if err != nil {
		return nil, err
	}
	args := []string{"list", "-f", "{{range .Deps}}{{.}}|{{end}}"}
	args = append(args, pkgs...)
	cmd := exec.Command("go", args...)
	cmd.Env = fixEnv(gopath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		output := string(out)
		if strings.Contains(output, "cannot find package") ||
			strings.Contains(output, "no buildable Go source files") {
			return nil, &MissingError{Err: output}
		}
		return nil, fmt.Errorf("'go %s' failed with:\n%s",
			strings.Join(args, " "), output)
	}
	deps := []string{}
	seen := map[string]bool{}
	for _, s := range strings.Split(string(out), "|") {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			deps = append(deps, s)
			seen[s] = true
		}
	}
	for _, pkg := range pkgs {
		if !seen[pkg] {
			seen[pkg] = true
			deps = append(deps, pkg)
		}
	}
	sort.Strings(deps)
	return deps, nil
}

// lists module dependencies
// direct determines whether or not to list indirect dependencies
func listModDependencies(includeIndirectDeps bool) ([]*PkgInfo, error) {
	args := []string{"list", "-m","-f","{{.Path}}|{{.Version}}|{{.Indirect}}|{{.Dir}}","all"}
	cmd := exec.Command("go", args...)
	out, err := cmd.CombinedOutput()
	output := string(out)
	if err != nil {
		if strings.Contains(output, "cannot find package") ||
			strings.Contains(output, "no buildable Go source files") {
			return nil, &MissingError{Err: output}
		}
		return nil, fmt.Errorf("'go %s' failed with:\n%s",
			strings.Join(args, " "), output)
	}
	var depInfos []*PkgInfo
	for _, dependency := range strings.Split(output, "\n"){
		// {{.Path}}|{{.Version}}|{{.Indirect}}|{{.Dir}}
		info :=  strings.Split(dependency, "|")
		anyEmpty := false
		for _, part := range info {
			if len(part) == 0 {
				anyEmpty = true
			}
		}
		if len(info) != 4 || anyEmpty {
			continue
		}
		indirectDep, err := strconv.ParseBool(info[2])
		if err != nil {
			return nil, fmt.Errorf("cannot parse boolean in dependency: %s", dependency)
		}
		if indirectDep && !includeIndirectDeps {
			continue
		}
		depInfo := &PkgInfo{
			Name: info[0],
			Version: info[1],
			ImportPath: info[0],
			Root: info[3],
		}
		depInfos = append(depInfos, depInfo)
	}
	return depInfos, nil
}

type PkgError struct {
	Err string
}

type PkgInfo struct {
	Name       string
	Dir        string
	Root       string
	ImportPath string
	Version    string
	Error      *PkgError
}

type Options struct {
	RunAll                  bool
	Words                   bool
	PrintConfidence         bool
	UseCsv                  bool
	UseMarkdown             bool
	IncludeIndirectDeps     bool
	PrunePath               string
	HelperListGlooPkgs      bool
	ConsolidatedLicenseFile string
	Pkgs                    []string
	Product                 Product
}

func PrintLicenses() error {
	opts := &Options{}
	flag.Usage = func() {
		fmt.Println(`Usage: licenses IMPORTPATH...

licenses lists all dependencies of specified packages or commands, excluding
standard library packages, and prints their licenses. Licenses are detected by
looking for files named like LICENSE, COPYING, COPYRIGHT and other variants in
the package directory, and its parent directories until one is found. Files
content is matched against a set of well-known licenses and the best match is
displayed along with its score.

With -a, all individual packages are displayed instead of grouping them by
license files.
With -w, words in package license file not found in the template license are
displayed. It helps assessing the changes importance.

Wrap PrintLicensesWithOptions with a go script if you would like to implement the Product interface.
The Product interface allows you to append or skip licenses.`)
		os.Exit(1)
	}
	flag.BoolVar(&opts.RunAll, "a", false, "display all individual packages")
	flag.BoolVar(&opts.Words, "w", false, "display words not matching license template")
	flag.BoolVar(&opts.PrintConfidence, "print-confidence", false, "display confidence level (default false)")
	flag.BoolVar(&opts.UseCsv, "csv", false, "print in csv format (default false)")
	flag.BoolVar(&opts.UseMarkdown, "markdown", false, "print in markdown table format (default false)")
	flag.StringVar(&opts.PrunePath, "prune-path", "", "prefix path to remove from the package and file specs during display output, ex: 'github.com/solo-io/gloo/vendor/'")
	flag.BoolVar(&opts.HelperListGlooPkgs, "helper-list-gloo-pkgs", false, "if set, will just print the list of packages concerning Gloo")
	flag.StringVar(&opts.ConsolidatedLicenseFile, "consolidated-license-file", "", "if set, will write all of the licenses' text to this file")
	opts.Pkgs = flag.Args()
	opts.Product = &genericProduct{}
	return PrintLicensesWithOptions(opts)

}
func PrintLicensesWithOptions(opts *Options) error {
	if opts.HelperListGlooPkgs {
		printGlooPkgNames()
		return nil
	}
	replacer := getPathReplacer(opts.Product.ReplacementList())
	if len(opts.Pkgs) < 1 {
		return fmt.Errorf("expect at least one package argument")
	}

	confidence := 0.7
	licenses, err := listLicenses(opts.Pkgs, opts.IncludeIndirectDeps)
	if err != nil {
		return err
	}
	if !opts.RunAll {
		licenses, err = groupLicenses(licenses)
		if err != nil {
			return err
		}
	}
	licenses = append(licenses, opts.Product.ExtraLicenses()...)
	w := tabwriter.NewWriter(os.Stdout, 1, 4, 2, ' ', 0)
	csvW := csv.NewWriter(os.Stdout)
	mdW := markdown.NewWriter(os.Stdout, []string{"Name", "Version", "License"})
	var includedLicenses []License
	for _, l := range licenses {
		license := "?"
		if l.Template != nil {
			if l.Score > .99 {
				license = fmt.Sprintf("%s", l.Template.Title)
				includedLicenses = append(includedLicenses, l)
			} else if l.Score >= confidence {
				includedLicenses = append(includedLicenses, l)
				if opts.PrintConfidence {
					license = fmt.Sprintf("%s (%2d%%)", l.Template.Title, int(100*l.Score))
				} else {
					license = fmt.Sprintf("%s", l.Template.Title)
				}
				if opts.Words && len(l.ExtraWords) > 0 {
					license += "\n\t+words: " + strings.Join(l.ExtraWords, ", ")
				}
				if opts.Words && len(l.MissingWords) > 0 {
					license += "\n\t-words: " + strings.Join(l.MissingWords, ", ")
				}
			} else {
				if opts.PrintConfidence {
					license = fmt.Sprintf("? (%s, %2d%%)", l.Template.Title, int(100*l.Score))
				} else {
					license = "UNKNOWN"
				}
			}
		} else if l.Err != "" {
			license = strings.Replace(l.Err, "\n", " ", -1)
		}
		packageString := l.Package
		pathString := l.ManualPath
		if pathString == "" {
			pathString = getPathString(l.Path, opts.PrunePath, replacer)
		}
		version := getVersion(pathString)
		if opts.PrunePath != "" {
			packageString = strings.TrimPrefix(packageString, opts.PrunePath)
		}
		license = opts.Product.OverrideLicense(packageString, license)
		if l.Template == nil {
			l.Template = &Template{
				Title: license,
			}
		}
		if opts.Product.SkipLicense(l) {
			continue
		}

		if opts.UseCsv {
			err = csvW.Write([]string{packageString, version, pathString, license})
		} else if opts.UseMarkdown {
			mdPackageLink := getMarkdownPackageLink(packageString)
			err = mdW.Write([]string{mdPackageLink, version, license})
		} else {
			_, err = w.Write([]byte(packageString + "\t" + license + "\n"))
		}
		if err != nil {
			return err
		}
	}
	if opts.ConsolidatedLicenseFile != "" {
		if err := writeConsolidatedLicenseFile(opts.ConsolidatedLicenseFile, includedLicenses); err != nil {
			return fmt.Errorf("unable to write consolidated license file %v", err)
		}
	}
	if opts.UseCsv {
		csvW.Flush()
		return nil
	}
	if opts.UseMarkdown {
		mdW.Flush()
		return nil
	}
	if err := w.Flush(); err != nil {
		return err
	}
	return nil
}

func getMarkdownPackageLink(packageString string) string {
	parts := strings.Split(packageString, "/")
	var shortPkgName string
	// get last two parts of package string for descriptive package name
	if len(parts) < 2 {
		shortPkgName = packageString
	} else {
		n := len(parts)
		shortPkgName = strings.Join(parts[n-2:n], "/")
	}
	// format github links
	if parts[0] == "github.com" && len(parts) > 2 {
		packageString = strings.Join(parts[:3], "/")
	}
	// format as markdown link
	return fmt.Sprintf("[%s](https://%s)", shortPkgName, packageString)
}

func getVersion(pathString string) string {
	regex := regexp.MustCompile("@(v.+)[\\/$]")
	matches := regex.FindStringSubmatch(pathString)
	if len(matches) < 1 {
		return "latest"
	}
	return matches[1]
}

func writeConsolidatedLicenseFile(outFile string, licenses []License) error {
	f, err := os.Create(outFile)
	if err != nil {
		return err
	}
	for i, l := range licenses {
		if _, err := f.WriteString(fmt.Sprintf("---\nIndex: %v\nPackage: %v\nLicense:\n", i, l.Package)); err != nil {
			return err
		}
		if _, err := f.Write(l.FileContent); err != nil {
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return nil
}

func getPathReplacer(oldnew []string) *strings.Replacer {
	return strings.NewReplacer(oldnew...)
}

// try to trim and substitute path elements, if available
func getPathString(rawPath, trimPrefix string, replacer *strings.Replacer) string {
	if trimPrefix == "" {
		return rawPath
	}
	pathString := strings.TrimPrefix(rawPath, trimPrefix)
	pathString = replacer.Replace(pathString)
	parts := strings.Split(pathString, "/")
	if len(parts) == 0 {
		return ""
	}
	switch {
	case len(parts) < 3:
		return pathString
	case parts[0] == "github.com":
		githubFilepathParts := []string{parts[0], parts[1], parts[2], "blob/master"}
		githubFilepathParts = append(githubFilepathParts, parts[3:]...)
		refinedString := strings.Join(githubFilepathParts, "/")
		return refinedString
	case parts[0] == "gopkg.in":
		// gopkg.in/pkg.v3      → github.com/go-pkg/pkg (branch/tag v3, v3.N, or v3.N.M)
		// gopkg.in/user/pkg.v3 → github.com/user/pkg   (branch/tag v3, v3.N, or v3.N.M)
		githubFilepathParts := []string{"github.com"}
		hasUserSpec := len(strings.Split(parts[1], ".")) == 1
		if hasUserSpec {
			userSpec := parts[1]
			pkgSpec := parts[2]
			pkgParts := strings.Split(pkgSpec, ".")
			if len(pkgParts) != 2 {
				return "UNKNOWN-version parse error"
			}
			version := pkgParts[1]
			githubFilepathParts = append(githubFilepathParts, userSpec, "blob", version)
			githubFilepathParts = append(githubFilepathParts, parts[3:]...)
			refinedString := strings.Join(githubFilepathParts, "/")
			return refinedString
		} else {
			pkgSpec := parts[1]
			pkgParts := strings.Split(pkgSpec, ".")
			if len(pkgParts) != 2 {
				return "UNKNOWN-version parse error"
			}
			pkgName := pkgParts[0]
			version := pkgParts[1]
			githubFilepathParts = append(githubFilepathParts, fmt.Sprintf("go-%v", pkgName), "blob", version)
			githubFilepathParts = append(githubFilepathParts, parts[2:]...)
			refinedString := strings.Join(githubFilepathParts, "/")
			return refinedString
		}
	}
	return pathString
}
