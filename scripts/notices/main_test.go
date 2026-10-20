package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoticeFilesAreTheLicensesAtAModulesRoot(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"LICENSE.txt", "NOTICE", "PATENTS", "COPYING.webpkit", "license.go", "README.md"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600))
	}
	require.NoError(t, os.Mkdir(filepath.Join(dir, "LICENSES"), 0o750))

	files, err := noticeFiles(dir)
	require.NoError(t, err)
	want := []string{"COPYING.webpkit", "LICENSE.txt", "NOTICE", "PATENTS"}
	for i, name := range want {
		want[i] = filepath.Join(dir, name)
	}
	assert.Equal(t, want, files)
}

func TestAModuleWithoutALicenseStopsTheNotices(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "NOTICE"), []byte("a notice, but no license"), 0o600))

	_, err := noticeFiles(dir)
	assert.ErrorIs(t, err, errNoLicense)
}

// TestTheNoticesCoverWhatTheServerLinks builds the notices of this checkout: Go's own, a module that only the Linux
// build links, a module with an Apache NOTICE, and the oEmbed list that embisto embeds.
func TestTheNoticesCoverWhatTheServerLinks(t *testing.T) {
	parts, err := collect(context.Background(), []string{"linux", "windows"})
	require.NoError(t, err)
	text, err := notices(parts)
	require.NoError(t, err)

	for _, heading := range []string{"Go standard library go", "github.com/prometheus/procfs v", "github.com/prometheus/client_golang v", "github.com/pantafive/embisto/internal/oembed\n"} {
		assert.Contains(t, text, strings.Repeat("=", 80)+"\n"+heading)
	}
	assert.Contains(t, text, "Copyright (c) 2016 Cal Henderson")
	assert.Contains(t, text, "Copyright 2009 The Go Authors.")
	assert.Contains(t, text, "Prometheus instrumentation library for Go applications")
}
