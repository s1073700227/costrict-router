package auth

import (
	"net/url"
	"testing"
)

func TestParseLoginURL(t *testing.T) {
	params, err := ParseLoginURL("https://www.abc.com/oidc-auth/api/v1/plugin/login?machine_code=m1&state=s1&provider=casdoor&plugin_version=2.8.1&vscode_version=1.120.0&uri_scheme=vscode")
	if err != nil {
		t.Fatal(err)
	}
	if params.BaseURL != "https://www.abc.com" {
		t.Fatalf("BaseURL = %q", params.BaseURL)
	}
	if params.MachineCode != "m1" || params.State != "s1" {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestBuildLoginURL(t *testing.T) {
	// 校验登录 URL 的路径和核心查询参数，防止插件登录协议被改坏。
	client := NewClient(LoginParams{
		BaseURL:     "https://www.abc.com",
		MachineCode: "m1",
		State:       "s1",
	}.WithDefaults())
	raw, err := client.BuildLoginURL()
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != LoginPath {
		t.Fatalf("path = %s", u.Path)
	}
	if u.Query().Get("machine_code") != "m1" || u.Query().Get("state") != "s1" {
		t.Fatalf("query = %s", u.RawQuery)
	}
}

func TestBuildLoginURLWithGeneratedValuesMatchesAPIDocShape(t *testing.T) {
	// 生成值必须符合 api.md 抓包形态，避免 machine_code/state 退回 UUID 格式。
	client := NewClient(LoginParams{
		BaseURL:     "https://www.abc.com",
		MachineCode: GenerateMachineCode(),
		State:       GenerateState(),
	}.WithDefaults())
	raw, err := client.BuildLoginURL()
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !ValidMachineCode(u.Query().Get("machine_code")) {
		t.Fatalf("machine_code = %q", u.Query().Get("machine_code"))
	}
	if got := u.Query().Get("state"); got == "" || got == "d62571f4-ab1e-41db-9a1b-0ad3be5fb0ef" {
		t.Fatalf("state = %q", got)
	}
}
