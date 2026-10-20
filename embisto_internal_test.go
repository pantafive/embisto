package embisto

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersionComesFromTheStampThenTheBuild(t *testing.T) {
	built := func(info *debug.BuildInfo) func() (*debug.BuildInfo, bool) {
		return func() (*debug.BuildInfo, bool) { return info, info != nil }
	}
	asMain := &debug.BuildInfo{Main: debug.Module{Path: modulePath, Version: "v0.2.0"}}
	asDependency := &debug.BuildInfo{Main: debug.Module{Path: "example.com/app", Version: "(devel)"}, Deps: []*debug.Module{
		{Path: "golang.org/x/net", Version: "v0.40.0"}, {Path: modulePath, Version: "v0.1.1-0.20261020093000-3562582abcde"},
	}}
	replaced := &debug.BuildInfo{Main: debug.Module{Path: "example.com/app"}, Deps: []*debug.Module{
		{Path: modulePath, Version: "v0.2.0", Replace: &debug.Module{Path: "../embisto"}},
	}}
	local := &debug.BuildInfo{Main: debug.Module{Path: modulePath, Version: "(devel)"}}

	assert.Equal(t, []string{"0.2.0-3-g3562582", "0.2.0", "0.1.1-0.20261020093000-3562582abcde", "devel", "devel", "devel"}, []string{
		versionOf("v0.2.0-3-g3562582", built(asMain)),
		versionOf("", built(asMain)),
		versionOf("", built(asDependency)),
		versionOf("", built(replaced)),
		versionOf("", built(local)),
		versionOf("", built(nil)),
	})
}
