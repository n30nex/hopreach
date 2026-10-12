package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"text/template"

	yconfig "hopreach/internal/config"
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

func TestCronFileContentPreservesGoResourceLimits(t *testing.T) {
	content, err := cronFileContent(yconfig.Default(), "/config/config.yaml", "1", "384MiB")
	if err != nil {
		t.Fatal(err)
	}
	want := "HOPREACH_CONFIG=/config/config.yaml\nGOMAXPROCS=1\nGOMEMLIMIT=384MiB\n" +
		"0 3 * * * root /app/hopreach >> /var/log/fetch.log 2>&1\n" +
		"17 4 * * * root /app/hopreach-shareapi -prune >> /var/log/prune.log 2>&1\n"
	if content != want {
		t.Fatalf("cron content = %q, want %q", content, want)
	}
}

func TestCronFileContentOmitsUnsetLimitsAndAcceptsRuntimeMemoryUnits(t *testing.T) {
	for _, memory := range []string{"", "off", "0", "402653184", "384MiB", "1B", "1KiB", "1GiB", "1TiB"} {
		t.Run(memory, func(t *testing.T) {
			content, err := cronFileContent(yconfig.Default(), "/config/config.yaml", "", memory)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(content, "GOMAXPROCS=") {
				t.Fatal("unset GOMAXPROCS must retain the runtime default")
			}
			if strings.Contains(content, "GOMEMLIMIT=") != (memory != "") {
				t.Fatalf("unexpected GOMEMLIMIT environment line: %q", content)
			}
		})
	}
}

func TestCronFileContentRejectsInjectedResourceValues(t *testing.T) {
	for _, value := range []string{"1\n* * * * * root injected", "1\rINJECTED=1", "$(injected)",
		"`injected`", "1; injected", "1%injected", "\"384MiB\"", "384 MiB", "1\x00"} {
		for _, variable := range []string{"GOMAXPROCS", "GOMEMLIMIT"} {
			t.Run(variable+value, func(t *testing.T) {
				procs, memory := "1", "384MiB"
				if variable == "GOMAXPROCS" {
					procs = value
				} else {
					memory = value
				}
				content, err := cronFileContent(yconfig.Default(), "/config/config.yaml", procs, memory)
				if err == nil || content != "" || !strings.Contains(err.Error(), variable) {
					t.Fatalf("unsafe resource value accepted: content=%q err=%v", content, err)
				}
			})
		}
	}
}
