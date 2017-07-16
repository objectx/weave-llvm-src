package main

import (
	"archive/tar"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"

	"github.com/pkg/errors"

	"xi2.org/x/xz"
)

var rxArchive = regexp.MustCompile(`(?P<stem>^.*)-(?P<version>\d+\.\d+\.\d+)\.src\.tar\.xz$`)

//type ArchiveSet struct {
//	version   string // Version string
//	llvm      string // LLVM itself
//	clang     string // CFE
//	lld       string
//	polly     string
//	compileRt string
//	openmp    string
//	libcxx    string
//	libcxxabi string
//	testSuite string
//}

type Archive struct {
	path    string
	name    string
	version string
}

func weave(dstRoot string, srcDir string) error {
	matched, err := collectArchives(srcDir)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "%s: Archives found:\n", ProgramName)
	for _, a := range matched {
		fmt.Fprintf(os.Stderr, "# %s\tversion: %s\tpath: %s\n", a.name, a.version, a.path)
	}
	err = expandArchives(dstRoot, matched)
	if err != nil {
		return err
	}
	return nil
}

// collectArchives collects all matched archives in `srcDir`
// Returned []Archive's 1st element is always LLVM core.
func collectArchives(srcDir string) ([]Archive, error) {
	d, err := ioutil.ReadDir(srcDir)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to scan \"%s\" as a source directory", srcDir)
	}
	var archives []Archive
	for _, f := range d {
		m := rxArchive.FindStringSubmatch(f.Name())
		if m == nil {
			verbose("%s: Mismatch %s\n", ProgramName, f.Name())
			continue
		}
		captures := intoNamedCapture(rxArchive, m)
		verbose("%s: Match %s (stem: %s, version: %s)\n",
			ProgramName, captures[""],
			captures["stem"], captures["version"])
		archives = append(archives,
			Archive{
				name:    captures["stem"],
				version: captures["version"],
				path:    filepath.Join(srcDir, captures[""]),
			})
	}
	if len(archives) == 0 {
		return nil, errors.Errorf("no matching archives found in \"%s\"", srcDir)
	}
	var llvm Archive
	{
		idx := findLLVMCore(archives)
		if idx < 0 {
			return nil, errors.Errorf("missing LLVM core archive in \"%s\"", srcDir)
		}
		llvm = archives[idx]
		archives = append(archives[:idx], archives[idx+1:]...)
	}
	matched := archives[:0]
	for _, a := range archives {
		if a.version != llvm.version {
			verbose("%s: \"%s\" rejected (version mismatch)\n", ProgramName, a.path)
		}
		matched = append(matched, a)
	}
	return append([]Archive{llvm}, matched...), nil
}

func intoNamedCapture(rx *regexp.Regexp, matches []string) map[string]string {
	result := make(map[string]string)
	for i, s := range rx.SubexpNames() {
		result[s] = matches[i]
	}
	return result
}

func findLLVMCore(archives []Archive) int {
	for i, a := range archives {
		if a.name == "llvm" {
			return i
		}
	}
	return -1
}

func expandArchives(dstRoot string, archives []Archive) error {
	for _, a := range archives {
		var path string
		switch a.name {
		case "llvm":
			path = filepath.Join(dstRoot, "llvm")
		case "cfe":
			path = filepath.Join(dstRoot, "llvm/tools/clang")
		case "clang-tools-extra":
			path = filepath.Join(dstRoot, "llvm/tools/clang/tools/extra")
		case "lld":
			path = filepath.Join(dstRoot, "llvm/tools/lld")
		case "lldb":
			path = filepath.Join(dstRoot, "llvm/tools/lldb")
		case "polly":
			path = filepath.Join(dstRoot, "llvm/tools/polly")
		case "compiler-rt":
			path = filepath.Join(dstRoot, "llvm/projects/compiler-rt")
		case "openmp":
			path = filepath.Join(dstRoot, "llvm/projects/openmp")
		case "libcxx":
			path = filepath.Join(dstRoot, "llvm/projects/libcxx")
		case "libcxxabi":
			path = filepath.Join(dstRoot, "llvm/projects/libcxxabi")
		case "test-suite":
			path = filepath.Join(dstRoot, "llvm/projects/test-suite")
		case "libunwind":
			path = filepath.Join(dstRoot, "llvm/projects/libunwind")
		default:
			return errors.Errorf("unknown component \"%s\" found", a.name)
		}
		if err := expandTarXz(path, a.path, fmt.Sprintf("%s-%s.src", a.name, a.version)); err != nil {
			return err
		}
	}
	return nil
}

// expanTarXz expands *.tar.xz file
func expandTarXz(dstDir string, inFile string, relRoot string) error {
	parentDir := filepath.Dir(dstDir)
	err := os.MkdirAll(parentDir, 0755)
	if err != nil {
		return errors.Wrapf(err, "failed to create \"%s\" as output directory", dstDir)
	}
	tmpDir, err := ioutil.TempDir(parentDir, "txz-")
	if err != nil {
		return errors.Wrapf(err, "faild to create \"%s\" as temporal output directory", tmpDir)
	}
	defer os.RemoveAll(tmpDir)
	input, err := os.Open(inFile)
	if err != nil {
		return err
	}
	defer input.Close()
	xzReader, err := xz.NewReader(input, xz.DefaultDictMax)
	if err != nil {
		return errors.Wrapf(err, "failed to create XZ format reader")
	}
	tarReader := tar.NewReader(io.Reader(xzReader))

	for {
		hdr, err := tarReader.Next()
		if err != nil {
			if err == io.EOF {
				break // The end
			}
			return errors.Wrapf(err, "failed to obtain tar header")
		}
		info := hdr.FileInfo()
		outPath, err := filepath.Rel(relRoot, hdr.Name)
		if err == nil {
			outPath = filepath.Join(tmpDir, outPath)
		} else {
			outPath = hdr.Name
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(outPath, info.Mode())
			if err != nil {
				return errors.Wrapf(err, "failed to create directory \"%s\"", outPath)
			}
		case tar.TypeReg, tar.TypeRegA:
			verbose("# %s", outPath)
			w, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
			if err != nil {
				return errors.Wrapf(err, "failed to create file \"%s\"", outPath)
			}
			_, err = io.Copy(w, tarReader)
			if err != nil {
				w.Close()
				return errors.Wrapf(err, "failed to copy contents to \"%s\"", outPath)
			}
			if err = w.Close(); err != nil {
				return errors.Wrapf(err, "failed to close file \"%s\"", outPath)
			}
		}
	}
	if err = input.Close(); err != nil {
		return errors.Wrapf(err, "failed to close \"%s\"", input.Name())
	}
	if exists(dstDir) {
		if err = os.RemoveAll(dstDir); err != nil {
			return errors.Wrapf(err, "failed to remove directory \"%s\"", dstDir)
		}
	}
	if err = os.Rename(tmpDir, dstDir); err != nil {
		errors.Wrapf(err, "failed to rename \"%s\" to \"%s\"", tmpDir, dstDir)
	}
	return nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
