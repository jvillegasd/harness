package install

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/jvillegasd/harness"
	"github.com/jvillegasd/harness/internal/checkout"
)

// assetsEnv names a directory to unpack instead of the embedded assets, so a test or a local edit installs a tree the binary was not built from.
const assetsEnv = "HARNESS_ASSETS"

// dataRoot is where an install unpacks the assets and what every managed link points into, so any directory can install or uninstall.
func (in *installer) dataRoot() checkout.Checkout {
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		data = filepath.Join(in.home, ".local", "share")
	}
	if !filepath.IsAbs(data) {
		die("XDG_DATA_HOME must be an absolute path; got %q.", data)
	}
	return checkout.Checkout{Root: filepath.Join(data, "harness")}
}

// unpack replaces the data root with this binary's assets. It builds beside the root and swaps it in, so a failed copy never leaves a half-written tree behind the links.
func (in *installer) unpack() {
	var assets fs.FS = harness.Assets
	if dir := os.Getenv(assetsEnv); dir != "" {
		assets = os.DirFS(dir)
	}
	root := in.dir.Root
	next := root + ".next"
	must(os.RemoveAll(next))
	must(os.MkdirAll(filepath.Dir(root), 0o755))
	if err := os.CopyFS(next, assets); err != nil {
		die("Unpacking the harness assets into %s failed: %v", next, err)
	}
	must(os.RemoveAll(root))
	must(os.Rename(next, root))
	in.log("Unpacked the harness into %s", root)
}

// installBinary copies the running binary to ~/.local/bin/harness, where the status line runs it, unless it already is that file.
func (in *installer) installBinary() {
	bin := filepath.Join(in.home, ".local", "bin", "harness")
	self, err := os.Executable()
	must(err)
	if self, err = filepath.EvalSymlinks(self); err != nil {
		die("Could not resolve the running binary: %v", err)
	}
	if resolved, err := filepath.EvalSymlinks(bin); err == nil && resolved == self {
		return
	}
	src, err := os.Open(self)
	must(err)
	defer src.Close()
	must(os.MkdirAll(filepath.Dir(bin), 0o755))
	tmp := bin + ".next"
	dst, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	must(err)
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		die("Copying harness to %s failed: %v", bin, err)
	}
	must(dst.Close())
	// A rename, so a status line drawing mid-install runs the old binary or the new one, never a partial file.
	must(os.Rename(tmp, bin))
	in.log("Installed %s", bin)
}
