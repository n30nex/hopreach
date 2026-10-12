package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"text/template"
)

func TestRegionalNginxDisablesLegacyGlobalPassthrough(t *testing.T) {
	b, err := os.ReadFile("../../docker/default.conf.template")
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := template.New("nginx").Parse(string(b))
	if err != nil {
		t.Fatal(err)
	}
	for _, regional := range []bool{false, true} {
		var out bytes.Buffer
		if err = tmpl.Execute(&out, nginxTemplateData{CoreScopeAPIURL: "https://upstream.invalid", Regional: regional}); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.String(), "proxy_pass https://upstream.invalid/") == regional {
			t.Fatalf("regional=%v did not enforce passthrough boundary", regional)
		}
	}
}
