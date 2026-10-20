// Command notices writes the license notices of the code built into embisto's server besides its own: the Go
// standard library's, each linked module's, and the NOTICE an embisto package keeps for a file it embeds from
// others. The release and the image run it, so the notices always match the build.
//
//	go run ./scripts/notices -o THIRD_PARTY_NOTICES
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// server is the package the notices are for.
const server = "github.com/pantafive/embisto/cmd/embisto"

var (
	// licenseNames and noticeNames start the names of the files that carry a module's notices; a module has at
	// least one of the first.
	licenseNames = []string{"LICENSE", "LICENCE", "COPYING"}
	noticeNames  = append([]string{"NOTICE", "PATENTS"}, licenseNames...)
)

var errNoLicense = errors.New("no license file")

// part is code under notices of its own.
type part struct {
	name  string
	files []string
}

// listed is what go list says of a package, in its names.
//
//nolint:tagliatelle // go list's names
type listed struct {
	ImportPath string `json:"ImportPath"`
	Dir        string `json:"Dir"`
	Standard   bool   `json:"Standard"`
	Module     *struct {
		Path    string `json:"Path"`
		Version string `json:"Version"`
		Dir     string `json:"Dir"` // the replacement's, when go.mod replaces the module
		Main    bool   `json:"Main"`
	} `json:"Module"`
}

func main() {
	out := flag.String("o", "THIRD_PARTY_NOTICES", "the file to write")
	systems := flag.String("goos", "linux,darwin,windows", "the systems whose builds the notices cover, separated by commas")
	flag.Parse()
	if err := run(context.Background(), *out, strings.Split(*systems, ",")); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, out string, systems []string) error {
	parts, err := collect(ctx, systems)
	if err != nil {
		return err
	}
	text, err := notices(parts)
	if err != nil {
		return err
	}
	if err := os.WriteFile(out, []byte(text), 0o644); err != nil { //nolint:gosec // a notice for everyone to read
		return fmt.Errorf("saving the notices: %w", err)
	}
	return nil
}

// collect finds the parts the server's build links on any of the systems: the standard library first, then the
// modules by path, then embisto's packages that keep a NOTICE.
func collect(ctx context.Context, systems []string) ([]part, error) {
	found := linked{modules: map[string]part{}, own: map[string]part{}}
	for _, goos := range systems {
		packages, err := list(ctx, goos)
		if err != nil {
			return nil, err
		}
		for _, pkg := range packages {
			if err := found.add(pkg); err != nil {
				return nil, err
			}
		}
	}
	goroot, err := toolchain(ctx)
	if err != nil {
		return nil, err
	}
	parts := []part{goroot}
	for _, path := range slices.Sorted(maps.Keys(found.modules)) {
		parts = append(parts, found.modules[path])
	}
	for _, path := range slices.Sorted(maps.Keys(found.own)) {
		parts = append(parts, found.own[path])
	}
	return parts, nil
}

// linked are the parts outside the standard library: modules by path, and embisto's packages by import path.
type linked struct{ modules, own map[string]part }

// add notes the part a package is in.
func (l linked) add(pkg listed) error {
	switch {
	case pkg.Standard || pkg.Module == nil:
	case pkg.Module.Main:
		notice := filepath.Join(pkg.Dir, "NOTICE")
		if _, err := os.Stat(notice); err == nil {
			l.own[pkg.ImportPath] = part{name: pkg.ImportPath, files: []string{notice}}
		}
	case l.modules[pkg.Module.Path].files != nil:
	default:
		files, err := noticeFiles(pkg.Module.Dir)
		if err != nil {
			return fmt.Errorf("%s: %w", pkg.Module.Path, err)
		}
		l.modules[pkg.Module.Path] = part{name: pkg.Module.Path + " " + pkg.Module.Version, files: files}
	}
	return nil
}

// list is every package the server's build on goos imports.
func list(ctx context.Context, goos string) ([]listed, error) {
	cmd := exec.CommandContext(ctx, "go", "list", "-deps", "-json", server)
	cmd.Env = append(os.Environ(), "GOOS="+goos)
	cmd.Stderr = os.Stderr
	raw, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("listing the packages for %s: %w", goos, err)
	}
	var packages []listed
	for decoder := json.NewDecoder(bytes.NewReader(raw)); ; {
		var pkg listed
		err := decoder.Decode(&pkg)
		if errors.Is(err, io.EOF) {
			return packages, nil
		}
		if err != nil {
			return nil, fmt.Errorf("reading the packages for %s: %w", goos, err)
		}
		packages = append(packages, pkg)
	}
}

// toolchain is the standard library of the Go that builds the server.
func toolchain(ctx context.Context) (part, error) {
	raw, err := exec.CommandContext(ctx, "go", "env", "-json", "GOROOT", "GOVERSION").Output()
	if err != nil {
		return part{}, fmt.Errorf("asking where Go is: %w", err)
	}
	var env struct {
		GOROOT    string `json:"GOROOT"`    //nolint:tagliatelle // go env's names
		GOVERSION string `json:"GOVERSION"` //nolint:tagliatelle // go env's names
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return part{}, fmt.Errorf("reading where Go is: %w", err)
	}
	license := filepath.Join(env.GOROOT, "LICENSE")
	if _, err := os.Stat(license); err != nil {
		license = filepath.Join(filepath.Dir(env.GOROOT), "LICENSE") // where Homebrew keeps it
	}
	return part{name: "Go standard library " + env.GOVERSION, files: []string{license, filepath.Join(env.GOROOT, "PATENTS")}}, nil
}

// noticeFiles are the license, notice and patent files at the root of a module, by name.
func noticeFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading the module: %w", err)
	}
	var files []string
	licensed := false
	for _, entry := range entries {
		name := strings.ToUpper(entry.Name())
		starts := func(prefix string) bool { return strings.HasPrefix(name, prefix) }
		if !entry.Type().IsRegular() || strings.HasSuffix(name, ".GO") || !slices.ContainsFunc(noticeNames, starts) {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
		licensed = licensed || slices.ContainsFunc(licenseNames, starts)
	}
	if !licensed {
		return nil, errNoLicense
	}
	return files, nil
}

// notices are each part's name and its files in full.
func notices(parts []part) (string, error) {
	var text strings.Builder
	text.WriteString("embisto is MIT-licensed (LICENSE). Its binary also holds the code below, each part under the notices that follow its name.\n")
	for _, p := range parts {
		text.WriteString("\n" + strings.Repeat("=", 80) + "\n" + p.name + "\n")
		for _, file := range p.files {
			raw, err := os.ReadFile(file) //nolint:gosec // a file of a module the build links
			if err != nil {
				return "", fmt.Errorf("%s: %w", p.name, err)
			}
			text.WriteString("\n" + strings.TrimSpace(string(raw)) + "\n")
		}
	}
	return text.String(), nil
}
