package talos

import (
	"strings"
	"testing"

	"bootimus/internal/kubernetes"
	"bootimus/internal/models"

	yaml "go.yaml.in/yaml/v3"
)

const baseWorkerConfig = `version: v1alpha1
machine:
  type: worker
  token: abc123
  install:
    disk: /dev/sda
    wipe: false
  kubelet:
    image: ghcr.io/siderolabs/kubelet:v1.33.0
cluster:
  controlPlane:
    endpoint: https://10.0.0.5:6443
`

func testNodeImage() *models.NodeImage {
	return &models.NodeImage{Name: "talos v1.10.5", Provider: models.ProviderTalos, Version: "v1.10.5", ImageRef: "abc123"}
}

func testCluster() *models.Cluster {
	return &models.Cluster{
		Name:               "prod",
		DefaultInstallDisk: "/dev/sda",
		WorkerConfig:       baseWorkerConfig,
	}
}

func renderConfig(t *testing.T, cluster *models.Cluster, client *models.Client) []byte {
	t.Helper()
	out, contentType, err := Provider{}.RenderConfig(cluster, client, "10.0.0.1")
	if err != nil {
		t.Fatalf("RenderConfig: %v", err)
	}
	if contentType != "application/yaml" {
		t.Fatalf("expected yaml content type, got %q", contentType)
	}
	return out
}

func parseYAML(t *testing.T, data []byte) map[string]interface{} {
	t.Helper()
	var doc map[string]interface{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("rendered config is not valid YAML: %v\n%s", err, data)
	}
	return doc
}

func dig(t *testing.T, doc map[string]interface{}, path ...string) interface{} {
	t.Helper()
	var cur interface{} = doc
	for _, key := range path {
		m, ok := cur.(map[string]interface{})
		if !ok {
			t.Fatalf("path %v: expected map, got %T", path, cur)
		}
		cur = m[key]
	}
	return cur
}

func TestRenderInjectsHostnameAndDisk(t *testing.T) {
	client := &models.Client{
		MACAddress:  "aa:bb:cc:dd:ee:ff",
		Name:        "worker-01",
		ClusterRole: models.ClusterRoleWorker,
		InstallDisk: "/dev/nvme0n1",
	}

	doc := parseYAML(t, renderConfig(t, testCluster(), client))

	if got := dig(t, doc, "machine", "network", "hostname"); got != "worker-01" {
		t.Errorf("expected hostname worker-01, got %v", got)
	}
	if got := dig(t, doc, "machine", "install", "disk"); got != "/dev/nvme0n1" {
		t.Errorf("expected client install disk override, got %v", got)
	}
	if got := dig(t, doc, "machine", "install", "wipe"); got != false {
		t.Errorf("expected sibling install keys to survive merge, got %v", got)
	}
	if got := dig(t, doc, "cluster", "controlPlane", "endpoint"); got != "https://10.0.0.5:6443" {
		t.Errorf("expected endpoint from uploaded config to pass through, got %v", got)
	}
}

func TestRenderHostnameFallbackAndDefaultDisk(t *testing.T) {
	client := &models.Client{MACAddress: "aa:bb:cc:dd:ee:ff", ClusterRole: models.ClusterRoleWorker}
	doc := parseYAML(t, renderConfig(t, testCluster(), client))

	if got := dig(t, doc, "machine", "network", "hostname"); got != "node-ddeeff" {
		t.Errorf("expected fallback hostname node-ddeeff, got %v", got)
	}
	if got := dig(t, doc, "machine", "install", "disk"); got != "/dev/sda" {
		t.Errorf("expected cluster default disk, got %v", got)
	}
}

func TestRenderAppliesConfigPatch(t *testing.T) {
	client := &models.Client{
		MACAddress:  "aa:bb:cc:dd:ee:ff",
		Name:        "worker-01",
		ClusterRole: models.ClusterRoleWorker,
		ConfigPatch: "machine:\n  nodeLabels:\n    rack: r7\n  kubelet:\n    extraArgs:\n      max-pods: \"200\"\n",
	}
	doc := parseYAML(t, renderConfig(t, testCluster(), client))

	if got := dig(t, doc, "machine", "nodeLabels", "rack"); got != "r7" {
		t.Errorf("expected patch to add node label, got %v", got)
	}
	if got := dig(t, doc, "machine", "kubelet", "extraArgs", "max-pods"); got != "200" {
		t.Errorf("expected patch to deep-merge kubelet args, got %v", got)
	}
	if got := dig(t, doc, "machine", "kubelet", "image"); got != "ghcr.io/siderolabs/kubelet:v1.33.0" {
		t.Errorf("expected base kubelet image to survive patch, got %v", got)
	}
	if got := dig(t, doc, "machine", "token"); got != "abc123" {
		t.Errorf("expected base token to survive patch, got %v", got)
	}
}

func TestRenderErrors(t *testing.T) {
	cluster := testCluster()

	if _, _, err := (Provider{}).RenderConfig(cluster, &models.Client{MACAddress: "aa", ClusterRole: "bogus"}, "10.0.0.1"); err == nil {
		t.Error("expected error for unknown role")
	}
	if _, _, err := (Provider{}).RenderConfig(cluster, &models.Client{MACAddress: "aa", ClusterRole: models.ClusterRoleControlPlane}, "10.0.0.1"); err == nil {
		t.Error("expected error for missing controlplane config")
	}
	if _, _, err := (Provider{}).RenderConfig(cluster, &models.Client{
		MACAddress:  "aa",
		ClusterRole: models.ClusterRoleWorker,
		ConfigPatch: ":\tnot yaml",
	}, "10.0.0.1"); err == nil {
		t.Error("expected error for invalid patch YAML")
	}
}

func TestValidateConfig(t *testing.T) {
	if !(Provider{}).ValidateConfig(baseWorkerConfig) {
		t.Error("expected valid talos config to pass")
	}
	if (Provider{}).ValidateConfig(`{"ignition":{"version":"3.4.0"}}`) {
		t.Error("expected ignition config to be rejected")
	}
	if (Provider{}).ValidateConfig("#cloud-config\nhostname: x\n") {
		t.Error("expected cloud-config to be rejected")
	}
}

func TestAssets(t *testing.T) {
	img := testNodeImage()
	assets, err := Provider{}.Assets(img)
	if err != nil {
		t.Fatalf("Assets: %v", err)
	}
	if len(assets) != 2 {
		t.Fatalf("expected 2 assets, got %d", len(assets))
	}
	if assets[0].URL != "https://factory.talos.dev/image/abc123/v1.10.5/kernel-amd64" {
		t.Errorf("unexpected kernel url %q", assets[0].URL)
	}
	if assets[0].Path != "talos/v1.10.5/abc123/vmlinuz" {
		t.Errorf("unexpected kernel path %q", assets[0].Path)
	}
	if assets[1].Path != "talos/v1.10.5/abc123/initramfs.xz" {
		t.Errorf("unexpected initramfs path %q", assets[1].Path)
	}

	img.Version = ""
	if _, err := (Provider{}).Assets(img); err == nil {
		t.Error("expected error when version missing")
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
		"kernel http://10.0.0.1:8080/node-images/talos/v1.10.5/abc123/vmlinuz",
		"talos.platform=metal",
		"talos.config=http://10.0.0.1:8080/machineconfig?m=${net0/mac}&u=${uuid}&t=tok",
		"initrd http://10.0.0.1:8080/node-images/talos/v1.10.5/abc123/initramfs.xz",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("expected script to contain %q:\n%s", want, script)
		}
	}
}

func TestConfigTemplate(t *testing.T) {
	cp := Provider{}.ConfigTemplate(models.ClusterRoleControlPlane)
	if !strings.Contains(cp, "type: controlplane") {
		t.Errorf("expected controlplane template, got:\n%s", cp)
	}
	if !(Provider{}).ValidateConfig(cp) {
		t.Error("expected controlplane template to be a valid talos config")
	}
	worker := Provider{}.ConfigTemplate(models.ClusterRoleWorker)
	if !strings.Contains(worker, "type: worker") {
		t.Errorf("expected worker template, got:\n%s", worker)
	}
}
