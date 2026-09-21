package scenarios

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validDeliveryDeclaration() map[string]any {
	return map[string]any{
		"treatments": []any{"inline", "native", "dispatch"},
		"contracts": []any{map[string]any{
			"id": "fix", "seed_tests": "seed/small_test.go", "acceptance_tests": "hidden/small_acceptance_test.go",
		}},
	}
}

func deliverySeedRoot(t *testing.T, files ...string) string {
	root := t.TempDir()
	for _, f := range files {
		path := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestValidateDeliveryDeclaration(t *testing.T) {
	root := deliverySeedRoot(t, "seed/small_test.go", "hidden/small_acceptance_test.go")
	if errs := validateDelivery(root, validDeliveryDeclaration()); len(errs) != 0 {
		t.Fatalf("valid declaration rejected: %v", errs)
	}
	cases := map[string]func(map[string]any){
		"unsupported arm":    func(d map[string]any) { d["treatments"] = []any{"inline", "native", "swarm"} },
		"missing arm":        func(d map[string]any) { d["treatments"] = []any{"inline", "native"} },
		"duplicate arm":      func(d map[string]any) { d["treatments"] = []any{"inline", "inline", "native", "dispatch"} },
		"malformed arms":     func(d map[string]any) { d["treatments"] = "inline" },
		"no contracts":       func(d map[string]any) { d["contracts"] = []any{} },
		"malformed contract": func(d map[string]any) { d["contracts"] = []any{"x"} },
		"missing id":         func(d map[string]any) { d["contracts"].([]any)[0].(map[string]any)["id"] = "" },
		"missing seed input": func(d map[string]any) { d["contracts"].([]any)[0].(map[string]any)["seed_tests"] = "seed/none_test.go" },
		"missing acceptance": func(d map[string]any) { delete(d["contracts"].([]any)[0].(map[string]any), "acceptance_tests") },
		"escaping path":      func(d map[string]any) { d["contracts"].([]any)[0].(map[string]any)["seed_tests"] = "../x" },
	}
	for name, edit := range cases {
		d := validDeliveryDeclaration()
		edit(d)
		if errs := validateDelivery(root, d); len(errs) == 0 {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestValidateDeliveryRejectsOutOfRootSymlink(t *testing.T) {
	root := deliverySeedRoot(t, "seed/seed_test.go", "hidden/acceptance_test.go")
	outside := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "seed", "link_test.go")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	declaration := validDeliveryDeclaration()
	contract := declaration["contracts"].([]any)[0].(map[string]any)
	contract["seed_tests"] = "seed/link_test.go"
	if errs := validateDelivery(root, declaration); len(errs) == 0 {
		t.Fatal("an out-of-root symlink was accepted")
	}
}

func TestDeliveryOracle(t *testing.T) {
	yes, no := true, false
	nativeEvent := deliveryEvent{Tool: "Agent", Subagent: "executor", Succeeded: true}
	dispatchEvent := deliveryEvent{Tool: "Bash", Command: "harness dispatch executor --prompt-file p", Succeeded: true}
	read := deliveryEvent{Tool: "Read"}
	cases := []struct {
		name string
		e    deliveryEvidence
		want string
	}{
		{"inline delivered", deliveryEvidence{Treatment: "inline", Available: true, Events: []deliveryEvent{read}, SeedPass: &yes, AcceptancePass: &yes}, deliveryDelivered},
		{"native delivered", deliveryEvidence{Treatment: "native", Available: true, Events: []deliveryEvent{nativeEvent}, SeedPass: &yes, AcceptancePass: &yes}, deliveryDelivered},
		{"dispatch delivered", deliveryEvidence{Treatment: "dispatch", Available: true, Events: []deliveryEvent{dispatchEvent}, SeedPass: &yes, AcceptancePass: &yes}, deliveryDelivered},
		{"inline failed", deliveryEvidence{Treatment: "inline", Available: true, Events: []deliveryEvent{read}, SeedPass: &yes, AcceptancePass: &no}, deliveryFailed},
		{"native failed", deliveryEvidence{Treatment: "native", Available: true, Events: []deliveryEvent{nativeEvent}, SeedPass: &no, AcceptancePass: &yes}, deliveryFailed},
		{"native executor failed", deliveryEvidence{Treatment: "native", Available: true, Events: []deliveryEvent{{Tool: "Agent", Subagent: "executor"}}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"dispatch failed", deliveryEvidence{Treatment: "dispatch", Available: true, Events: []deliveryEvent{dispatchEvent}, SeedPass: &yes, AcceptancePass: &no}, deliveryFailed},
		{"inline spawned agent", deliveryEvidence{Treatment: "inline", Available: true, Events: []deliveryEvent{nativeEvent}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"inline dispatched", deliveryEvidence{Treatment: "inline", Available: true, Events: []deliveryEvent{dispatchEvent}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"inline ignores non-Bash metadata", deliveryEvidence{Treatment: "inline", Available: true, Events: []deliveryEvent{{Tool: "Read", Command: "harness dispatch executor --prompt-file p", Succeeded: true}}, SeedPass: &yes, AcceptancePass: &yes}, deliveryDelivered},
		{"inline failed dispatch", deliveryEvidence{Treatment: "inline", Available: true, Events: []deliveryEvent{{Tool: "Bash", Command: "harness dispatch executor --prompt-file p", Succeeded: false}}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"native failed dispatch", deliveryEvidence{Treatment: "native", Available: true, Events: []deliveryEvent{nativeEvent, {Tool: "Bash", Command: "harness dispatch executor --prompt-file p", Succeeded: false}}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"inline wrapped dispatch", deliveryEvidence{Treatment: "inline", Available: true, Events: []deliveryEvent{{Tool: "Bash", Command: "env harness dispatch executor --prompt-file p", Succeeded: true}}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"native wrapped dispatch", deliveryEvidence{Treatment: "native", Available: true, Events: []deliveryEvent{nativeEvent, {Tool: "Bash", Command: "env harness dispatch executor --prompt-file p", Succeeded: true}}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"native without event", deliveryEvidence{Treatment: "native", Available: true, Events: []deliveryEvent{read}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"native other subagent", deliveryEvidence{Treatment: "native", Available: true, Events: []deliveryEvent{{Tool: "Agent", Subagent: "Explore"}}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"dispatch without command", deliveryEvidence{Treatment: "dispatch", Available: true, Events: []deliveryEvent{read}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"dispatch command not successful", deliveryEvidence{Treatment: "dispatch", Available: true, Events: []deliveryEvent{{Tool: "Bash", Command: "harness dispatch executor --prompt-file p", Succeeded: false}}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"dispatch command is only echoed", deliveryEvidence{Treatment: "dispatch", Available: true, Events: []deliveryEvent{{Tool: "Bash", Command: "echo harness dispatch executor", Succeeded: true}}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"dispatch is not a non-Bash event", deliveryEvidence{Treatment: "dispatch", Available: true, Events: []deliveryEvent{{Tool: "Read", Command: "harness dispatch executor", Succeeded: true}}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"fake harness executable", deliveryEvidence{Treatment: "dispatch", Available: true, Events: []deliveryEvent{{Tool: "Bash", Command: "fakeharness dispatch executor", Succeeded: true}}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"untrusted harness path", deliveryEvidence{Treatment: "dispatch", Available: true, Events: []deliveryEvent{{Tool: "Bash", Command: "/tmp/fake/harness dispatch executor", Succeeded: true}}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"compound dispatch command", deliveryEvidence{Treatment: "dispatch", Available: true, Events: []deliveryEvent{{Tool: "Bash", Command: "harness dispatch executor || true", Succeeded: true}}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"dispatch extra arguments", deliveryEvidence{Treatment: "dispatch", Available: true, Events: []deliveryEvent{{Tool: "Bash", Command: "harness dispatch executor --prompt-file p --unexpected x", Succeeded: true}}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"dispatch newline command", deliveryEvidence{Treatment: "dispatch", Available: true, Events: []deliveryEvent{{Tool: "Bash", Command: "harness dispatch executor --prompt-file p\ntrue", Succeeded: true}}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"dispatch repeated spaces", deliveryEvidence{Treatment: "dispatch", Available: true, Events: []deliveryEvent{{Tool: "Bash", Command: "harness  dispatch executor --prompt-file p", Succeeded: true}}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"inline escaped newline dispatch", deliveryEvidence{Treatment: "inline", Available: true, Events: []deliveryEvent{{Tool: "Bash", Command: "harness dispatch \\\nexecutor --prompt-file p", Succeeded: true}}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"dispatch command substitution", deliveryEvidence{Treatment: "dispatch", Available: true, Events: []deliveryEvent{{Tool: "Bash", Command: "harness dispatch executor --prompt-file p$(true)", Succeeded: true}}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"dispatch backtick expansion", deliveryEvidence{Treatment: "dispatch", Available: true, Events: []deliveryEvent{{Tool: "Bash", Command: "harness dispatch executor --prompt-file `true`", Succeeded: true}}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"inline executor expansion", deliveryEvidence{Treatment: "inline", Available: true, Events: []deliveryEvent{{Tool: "Bash", Command: "harness dispatch executor$(true) --prompt-file p", Succeeded: true}}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"quoted dispatch text", deliveryEvidence{Treatment: "inline", Available: true, Events: []deliveryEvent{{Tool: "Bash", Command: "printf '%s' 'harness dispatch \\\nexecutor --prompt-file p'", Succeeded: true}}, SeedPass: &yes, AcceptancePass: &yes}, deliveryDelivered},
		{"inline echoed dispatch words", deliveryEvidence{Treatment: "inline", Available: true, Events: []deliveryEvent{{Tool: "Bash", Command: "echo dispatch executor", Succeeded: true}}, SeedPass: &yes, AcceptancePass: &yes}, deliveryDelivered},
		{"inline eval dispatch", deliveryEvidence{Treatment: "inline", Available: true, Events: []deliveryEvent{{Tool: "Bash", Command: "eval \"harness dispatch executor --prompt-file p\"", Succeeded: true}}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"inline shell boundary is not dispatch", deliveryEvidence{Treatment: "inline", Available: true, Events: []deliveryEvent{{Tool: "Bash", Command: "harness dispatch; executor", Succeeded: true}}, SeedPass: &yes, AcceptancePass: &yes}, deliveryDelivered},
		{"inline dispatch followed by shell command", deliveryEvidence{Treatment: "inline", Available: true, Events: []deliveryEvent{{Tool: "Bash", Command: "harness dispatch executor --prompt-file p; true", Succeeded: true}}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"wrapped dispatch command", deliveryEvidence{Treatment: "inline", Available: true, Events: []deliveryEvent{{Tool: "Bash", Command: "bash -c 'harness dispatch executor --prompt-file p'", Succeeded: true}}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"conflicting dispatch attempts", deliveryEvidence{Treatment: "dispatch", Available: true, Events: []deliveryEvent{dispatchEvent, {Tool: "Bash", Command: "harness dispatch executor --prompt-file p", Succeeded: false}}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"native and dispatch evidence conflict", deliveryEvidence{Treatment: "native", Available: true, Events: []deliveryEvent{nativeEvent, dispatchEvent}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"native extra delegation", deliveryEvidence{Treatment: "native", Available: true, Events: []deliveryEvent{nativeEvent, {Tool: "Agent", Subagent: "reviewer-a"}}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"dispatch extra delegation", deliveryEvidence{Treatment: "dispatch", Available: true, Events: []deliveryEvent{dispatchEvent, nativeEvent}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"dispatch narrated only", deliveryEvidence{Treatment: "dispatch", Available: true, Narration: "I ran harness dispatch executor", SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"unavailable route", deliveryEvidence{Treatment: "inline", Available: false, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"missing acceptance", deliveryEvidence{Treatment: "inline", Available: true, SeedPass: &yes}, deliveryUnverified},
		{"missing events", deliveryEvidence{Treatment: "inline", Available: true, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"empty events", deliveryEvidence{Treatment: "inline", Available: true, Events: []deliveryEvent{}, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"missing seed", deliveryEvidence{Treatment: "inline", Available: true, AcceptancePass: &yes}, deliveryUnverified},
		{"unknown treatment", deliveryEvidence{Treatment: "swarm", Available: true, SeedPass: &yes, AcceptancePass: &yes}, deliveryUnverified},
		{"non-adherent failing", deliveryEvidence{Treatment: "native", Available: true, SeedPass: &no, AcceptancePass: &no}, deliveryUnverified},
	}
	for _, c := range cases {
		got, reasons := deliveryOracle(c.e)
		if got != c.want {
			t.Errorf("%s: got %s (%s), want %s", c.name, got, strings.Join(reasons, "; "), c.want)
		}
		if got != deliveryDelivered && len(reasons) == 0 {
			t.Errorf("%s: no reasons", c.name)
		}
	}
}

func TestDeliveryProbeUsesRecordedFixtures(t *testing.T) {
	artifact, err := probeDeliveryRun(testRepoRoot())
	if err != nil {
		t.Fatal(err)
	}
	fixtures, ok := artifact["fixtures"].(map[string]any)
	if !ok {
		t.Fatalf("fixtures = %#v", artifact["fixtures"])
	}
	want := map[string]string{
		"inline-delivered":      deliveryDelivered,
		"native-delivered":      deliveryDelivered,
		"dispatch-delivered":    deliveryDelivered,
		"inline-failed":         deliveryFailed,
		"inline-non-adherent":   deliveryUnverified,
		"native-non-adherent":   deliveryUnverified,
		"dispatch-non-adherent": deliveryUnverified,
		"route-unavailable":     deliveryUnverified,
		"missing-evidence":      deliveryUnverified,
	}
	for name, expected := range want {
		entry, _ := fixtures[name].(map[string]any)
		if got := getString(entry, "outcome"); got != expected {
			t.Errorf("%s outcome = %q, want %q", name, got, expected)
		}
	}
}

func TestDeliveryMatrixValidatesSeedInputs(t *testing.T) {
	root := testRepoRoot()
	matrix := loadTestMatrixAt(t, filepath.Join(root, "scenarios", "matrix.delivery.json"))
	if errs := validateMatrixAt(root, matrix); len(errs) != 0 {
		t.Fatalf("delivery matrix errors: %v", errs)
	}
}
