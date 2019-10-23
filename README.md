Fork of https://github.com/pmezard/licenses

- Modified to include:
  - CSV output
  - Link to actual license
  - Option to output a list of all licenses
  - Export core function
  - Adaptations for ease of use in https://github.com/solo-io/gloo
    - May add other product-specific helpers in the future

# Usage

## Import into another binary

- define `Options` instead of passing flags
```go
import "github.com/solo-io/go-list-licenses/pkg/license"

func run() error {
	glooOptions := &license.Options{
		RunAll:                  false,
		Words:                   false,
		PrintConfidence:         false,
		UseCsv:                  true,
		PrunePath:               "github.com/solo-io/gloo/vendor/",
		HelperListGlooPkgs:      false,
		ConsolidatedLicenseFile: "third_party_licenses.txt",
		ProductName:             "gloo",
		Pkgs: []string{
			"github.com/solo-io/gloo/projects/accesslogger/cmd",
			"github.com/solo-io/gloo/projects/discovery/cmd",
			"github.com/solo-io/gloo/projects/envoyinit/cmd",
			"github.com/solo-io/gloo/projects/gateway/cmd",
			"github.com/solo-io/gloo/projects/gloo/cmd",
			"github.com/solo-io/gloo/projects/ingress/cmd",
			"github.com/solo-io/gloo/projects/hypergloo",
		},
	}
	return license.PrintLicensesWithOptions(glooOptions)
}
```

## Run as a script with commandline flags
- must be run from within $GOROOT
  - For analyzing go mod projects, consider using https://github.com/mitchellh/golicense instead
```bash
# first compile with go build -o analyze-licenses main.go
# cd to the gloo directory
# run the command on all gloo binaries:
~/git/github.com/solo-io/go-list-licenses/analyze-licenses \
    -prune-path github.com/solo-io/gloo/vendor/ \
    -csv \
    -consolidated-license-file third_party_license_list.txt
    github.com/solo-io/gloo/projects/accesslogger/cmd \
    github.com/solo-io/gloo/projects/discovery/cmd \
    github.com/solo-io/gloo/projects/envoyinit/cmd \
    github.com/solo-io/gloo/projects/gateway/cmd \
    github.com/solo-io/gloo/projects/gloo/cmd \
    github.com/solo-io/gloo/projects/ingress/cmd \
    github.com/solo-io/gloo/projects/hypergloo
```




---

original readme below:

# What is it?

`licenses` uses `go list` tool over a Go workspace to collect the dependencies
of a package or command, detect their license if any and match them against
well-known templates.

```
$ licenses github.com/blevesearch/bleve
github.com/blevesearch/bleve             Apache License 2.0
github.com/blevesearch/go-porterstemmer  MIT License (93%)
github.com/blevesearch/segment           Apache License 2.0
github.com/boltdb/bolt                   MIT License (97%)
github.com/golang/protobuf/proto         BSD 3-clause "New" or "Revised" License (91%)
github.com/steveyen/gtreap               MIT License (96%)
vendor/golang.org/x/net/http2/hpack      ?
```

Unmatched license words can be displayed with:
```
$ licenses -w github.com/steveyen/gtreap
github.com/steveyen/gtreap  MIT License (98%)
                            -words: mit, license
```

# Where does it come from?

Both the code and reference data were directly ported from:

  [https://github.com/benbalter/licensee](https://github.com/benbalter/licensee)
