package install

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// ReconcileMCP refreshes one server registration for one installed client. It
// never runs setup, installs assets, changes tracked settings, or starts clients.
func ReconcileMCP(client, server, binary string, stdout, stderr io.Writer) (err error) {
	defer func() {
		if p := recover(); p != nil {
			if f, ok := p.(fatal); ok {
				err = fmt.Errorf("%s", f)
			} else {
				panic(p)
			}
		}
	}()
	in := &installer{stdout: stdout, stderr: stderr}
	in.resolveLocations()
	if _, err := exec.LookPath(client); err != nil {
		return fmt.Errorf("client %s is missing", client)
	}
	return in.registerMCP(client, server, binary)
}

func (in *installer) registerMCP(client, server, binary string) error {
	args := []string{binary, "mcp"}
	if server == "engram" {
		args = append(args, "--tools=agent")
	}
	switch client {
	case "claude":
		_ = exec.Command("claude", "mcp", "remove", "--scope", "user", server).Run()
		cmd := exec.Command("claude", append([]string{"mcp", "add", "--scope", "user", server, "--"}, args...)...)
		cmd.Stdout, cmd.Stderr = in.stdout, in.stderr
		return cmd.Run()
	case "codex":
		_ = in.codex("mcp", "remove", server).Run()
		return in.codex(append([]string{"mcp", "add", server, "--"}, args...)...).Run()
	case "opencode":
		path, doc, servers := in.openCodeConfig()
		servers.set(server, encode(struct {
			Command []string `json:"command"`
			Enabled bool     `json:"enabled"`
			Type    string   `json:"type"`
		}{args, true, "local"}))
		doc.set("mcp", encode(servers))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		return os.WriteFile(path, render(doc), 0644)
	default:
		return fmt.Errorf("unsupported MCP client %s", client)
	}
}
