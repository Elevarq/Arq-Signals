package tests

import (
	"strings"
	"testing"
)

// #347 — the chart renders multiple PostgreSQL targets from a
// `.Values.targets` list, so one Signals install can monitor several
// databases (e.g. primary + replica + analytics). The single
// `.Values.target` block is preserved unchanged for back-compat.
// Spec: specifications/helm_multi_target.md.

// HMT-R001/HMT-R002 — every list entry renders, with field-name
// translation (authMethod -> auth_method, sslRootCertFile ->
// sslrootcert_file).
func TestHelm_MultiTarget_RendersEachEntry(t *testing.T) {
	out := renderHelm(t,
		"targets[0].name=replica",
		"targets[0].host=replica.example",
		"targets[0].sslmode=verify-full",
		"targets[0].authMethod=aws_rds_iam",
		"targets[0].region=us-east-1",
		"targets[0].sslRootCertFile=/etc/signals/server-ca.crt",
		"targets[1].name=analytics",
		"targets[1].host=analytics.example",
		"targets[1].sslmode=verify-full",
		"targets[1].authMethod=secret_store",
		"targets[1].secretRef=arn:aws:secretsmanager:us-east-1:1:secret:analytics",
	)

	for _, want := range []string{
		"targets:",
		"- name: replica",
		"host: replica.example",
		"auth_method: aws_rds_iam",
		"region: us-east-1",
		"sslrootcert_file: /etc/signals/server-ca.crt",
		"- name: analytics",
		"host: analytics.example",
		"auth_method: secret_store",
		"secret_ref: arn:aws:secretsmanager:us-east-1:1:secret:analytics",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("multi-target render missing %q:\n%s", want, out)
		}
	}
}

// HMT-R003 — when both the single target and the list are set, the
// `default` entry renders first, then the list entries.
func TestHelm_MultiTarget_DefaultPlusList(t *testing.T) {
	out := renderHelm(t,
		"target.host=primary.example",
		"targets[0].name=replica",
		"targets[0].host=replica.example",
	)
	if !strings.Contains(out, "- name: default") {
		t.Errorf("single target `default` entry missing when both set:\n%s", out)
	}
	if !strings.Contains(out, "- name: replica") {
		t.Errorf("list entry `replica` missing when both set:\n%s", out)
	}
	di := strings.Index(out, "- name: default")
	ri := strings.Index(out, "- name: replica")
	if di < 0 || ri < 0 || di > ri {
		t.Errorf("expected `default` to render before `replica` (default=%d replica=%d)", di, ri)
	}
}

// HMT-R006 — a list entry's file/env password source renders.
func TestHelm_MultiTarget_PasswordFileAndEnv(t *testing.T) {
	out := renderHelm(t,
		"targets[0].name=pw",
		"targets[0].host=pw.example",
		"targets[0].passwordFile=/etc/signals/pg_password",
		"targets[1].name=pwenv",
		"targets[1].host=pwenv.example",
		"targets[1].passwordEnv=PW_ENV",
	)
	for _, want := range []string{
		"password_file: /etc/signals/pg_password",
		"password_env: PW_ENV",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("multi-target password source missing %q:\n%s", want, out)
		}
	}
}

// INV-HMT-01 — with only the single target set (no `targets`), exactly
// one target entry renders and it is the `default` one. Guards the
// byte-stable back-compat path for existing installs.
func TestHelm_SingleTarget_StillOneDefaultEntry(t *testing.T) {
	out := renderHelm(t, "target.host=primary.example")
	if n := strings.Count(configSignalsYAML(out), "- name:"); n != 1 {
		t.Errorf("single-target install rendered %d target entries in signals.yaml, want exactly 1:\n%s", n, out)
	}
	if !strings.Contains(out, "- name: default") {
		t.Errorf("single-target `default` entry missing:\n%s", out)
	}
}

// HMT-R005 — no target and no list -> no `targets:` block in signals.yaml.
func TestHelm_NoTargets_OmitsTargetsBlock(t *testing.T) {
	body := configSignalsYAML(renderHelm(t))
	if strings.Contains(body, "targets:") {
		t.Errorf("no target configured, but signals.yaml has a targets block:\n%s", body)
	}
}

// configSignalsYAML returns just the ConfigMap's signals.yaml payload so
// assertions on target entries are not confused by `- name:` container /
// volume entries elsewhere in the render.
func configSignalsYAML(out string) string {
	const marker = "signals.yaml: |"
	i := strings.Index(out, marker)
	if i < 0 {
		return ""
	}
	rest := out[i+len(marker):]
	// The block ends at the next resource separator.
	if j := strings.Index(rest, "\n---"); j >= 0 {
		return rest[:j]
	}
	return rest
}
