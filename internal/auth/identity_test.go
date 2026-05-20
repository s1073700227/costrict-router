package auth

import (
	"regexp"
	"testing"
)

func TestGenerateMachineCodeMatchesPluginShape(t *testing.T) {
	code := GenerateMachineCode()
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(code) {
		t.Fatalf("machine_code 格式不正确: %q", code)
	}
	if !ValidMachineCode(code) {
		t.Fatalf("ValidMachineCode returned false for %q", code)
	}
}

func TestGenerateStateMatchesPluginShape(t *testing.T) {
	state := GenerateState()
	if !regexp.MustCompile(`^[a-z0-9]{16,32}$`).MatchString(state) {
		t.Fatalf("state 格式不正确: %q", state)
	}
}
