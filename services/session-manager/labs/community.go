package labs

import (
	"embed"
	"fmt"
	"path"
	"sort"
)

// community holds hand-written lab specs. Each one must pass Validate and,
// in CI, ValidateInSandbox (see labs_test.go and cmd/labctl).
//
//go:embed community/*.json
var community embed.FS

// CommunitySpecs returns the embedded specs, sorted by file name.
func CommunitySpecs() ([]Spec, error) {
	entries, err := community.ReadDir("community")
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var out []Spec
	for _, e := range entries {
		data, err := community.ReadFile(path.Join("community", e.Name()))
		if err != nil {
			return nil, err
		}
		s, err := ParseSpec(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		if problems := s.Validate(); len(problems) > 0 {
			return nil, fmt.Errorf("%s: %v", e.Name(), problems)
		}
		out = append(out, s)
	}
	return out, nil
}

func init() {
	specs, err := CommunitySpecs()
	if err != nil {
		// A broken community spec is a build defect; the unit test fails on
		// it, and the service starts with the built-in catalog only.
		return
	}
	for _, s := range specs {
		Register(s.Lab())
	}
}
