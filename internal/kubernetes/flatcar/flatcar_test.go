package flatcar

import (
	"encoding/json"
	"strings"
	"testing"

	"bootimus/internal/kubernetes"
	"bootimus/internal/models"
)

const baseWorkerIgnition = `{
  "ignition": {"version": "3.4.0"},
  "storage": {
    "files": [
      {"path": "/etc/hostname", "contents": {"source": "data:,{{HOSTNAME}}"}}
    ]
  },
  "systemd": {
    "units": [
      {"name": "k3s-agent.service", "enabled": true, "contents": "[Service]\nExecStart=/opt/k3s agent\n"}
    ]
  }
}`

const talosWorkerConfig = "version: v1alpha1\nmachine:\n  type: worker\n"

func testNodeImage() *models.NodeImage {
	return &models.NodeImage{Name: "flatcar stable", Provider: models.ProviderFlatcar, Version: "current", ImageRef: "stable"}
}

func testCluster() *models.Cluster {
	return &models.Cluster{Name: "edge", WorkerConfig: baseWorkerIgnition}
}

func TestValidateConfig(t *testing.T) {
	if !(Provider{}).ValidateConfig(baseWorkerIgnition) {
		t.Error("expected ignition config to validate")
	}
	if (Provider{}).ValidateConfig(talosWorkerConfig) {
		t.Error("expected talos config to be rejected")
	}
	if (Provider{}).ValidateConfig(`{"foo": "bar"}`) {
		t.Error("expected json without ignition key to be rejected")
	}
	if (Provider{}).ValidateConfig("#cloud-config\nhostname: x\n") {
		t.Error("expected cloud-config to be rejected")
	}
}

func TestAssets(t *testing.T) {
	assets, err := Provider{}.Assets(testNodeImage())
	if err != nil {
		t.Fatalf("Assets: %v", err)
	}
	if len(assets) != 2 {
		t.Fatalf("expected 2 assets, got %d", len(assets))
	}
	if assets[0].URL != "https://stable.release.flatcar-linux.net/amd64-usr/current/flatcar_production_pxe.vmlinuz" {
		t.Errorf("unexpected kernel url %q", assets[0].URL)
	}
	if assets[0].Path != "flatcar/current/stable/vmlinuz" {
		t.Errorf("unexpected kernel path %q", assets[0].Path)
	}
	if assets[1].Path != "flatcar/current/stable/initrd.cpio.gz" {
		t.Errorf("unexpected initrd path %q", assets[1].Path)
	}

	empty := &models.NodeImage{Name: "fresh", Provider: models.ProviderFlatcar}
	assets, err = Provider{}.Assets(empty)
	if err != nil || assets[0].Path != "flatcar/current/stable/vmlinuz" {
		t.Errorf("expected stable/current defaults, got %v %v", assets, err)
	}
}

func TestInstallScript(t *testing.T) {
	client := &models.Client{MACAddress: "aa:bb:cc:dd:ee:ff", Name: "worker-01", ClusterRole: models.ClusterRoleWorker}
	ctx := kubernetes.BootContext{
		AssetBaseURL: "http://10.0.0.1:8080/node-images",
		ConfigURL:    "http://10.0.0.1:8080/machineconfig?m=${net0/mac}&u=${uuid}&t=tok",
	}

	script, err := Provider{}.InstallScript(testNodeImage(), testCluster(), client, ctx)
	if err != nil {
		t.Fatalf("InstallScript: %v", err)
	}
	for _, want := range []string{
		"#!ipxe",
		"kernel http://10.0.0.1:8080/node-images/flatcar/current/stable/vmlinuz",
		"flatcar.first_boot=1",
		"ignition.config.url=http://10.0.0.1:8080/machineconfig?m=${net0/mac}&u=${uuid}&t=tok",
		"initrd http://10.0.0.1:8080/node-images/flatcar/current/stable/initrd.cpio.gz",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("expected script to contain %q:\n%s", want, script)
		}
	}
}

func TestRenderConfig(t *testing.T) {
	client := &models.Client{
		MACAddress:  "aa:bb:cc:dd:ee:ff",
		Name:        "worker-01",
		ClusterRole: models.ClusterRoleWorker,
		ConfigPatch: "passwd:\n  users:\n    - name: core\n",
	}

	out, contentType, err := Provider{}.RenderConfig(testCluster(), client, "10.0.0.1")
	if err != nil {
		t.Fatalf("RenderConfig: %v", err)
	}
	if contentType != "application/json" {
		t.Errorf("expected json content type, got %q", contentType)
	}

	var doc map[string]interface{}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("rendered config is not valid JSON: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "data:,worker-01") {
		t.Errorf("expected hostname placeholder substitution:\n%s", out)
	}
	if _, ok := doc["passwd"]; !ok {
		t.Errorf("expected patch to merge into config:\n%s", out)
	}
	if _, ok := doc["systemd"]; !ok {
		t.Errorf("expected base systemd units to survive:\n%s", out)
	}
}

func TestRenderConfigErrors(t *testing.T) {
	cluster := testCluster()

	if _, _, err := (Provider{}).RenderConfig(cluster, &models.Client{MACAddress: "aa", ClusterRole: "bogus"}, "10.0.0.1"); err == nil {
		t.Error("expected error for unknown role")
	}
	if _, _, err := (Provider{}).RenderConfig(cluster, &models.Client{MACAddress: "aa", ClusterRole: models.ClusterRoleControlPlane}, "10.0.0.1"); err == nil {
		t.Error("expected error for missing controlplane config")
	}

	cluster.WorkerConfig = "not json at all"
	if _, _, err := (Provider{}).RenderConfig(cluster, &models.Client{MACAddress: "aa", ClusterRole: models.ClusterRoleWorker}, "10.0.0.1"); err == nil {
		t.Error("expected error for invalid JSON config")
	}
}

func TestConfigTemplateValidates(t *testing.T) {
	tmpl := Provider{}.ConfigTemplate(models.ClusterRoleWorker)
	if !(Provider{}).ValidateConfig(tmpl) {
		t.Errorf("expected config template to be valid ignition:\n%s", tmpl)
	}
}
