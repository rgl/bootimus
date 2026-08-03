package talos

import (
	"fmt"
	"net/url"
	"strings"

	"bootimus/internal/kubernetes"
	"bootimus/internal/models"

	yaml "go.yaml.in/yaml/v3"
)

const factoryBaseURL = "https://factory.talos.dev/image"

const vanillaSchematicID = "376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba"

type Provider struct{}

func init() {
	kubernetes.RegisterProvider(Provider{})
}

func (Provider) Name() string {
	return models.ProviderTalos
}

func (Provider) Info() kubernetes.ProviderInfo {
	return kubernetes.ProviderInfo{
		Name:           models.ProviderTalos,
		Label:          "Talos Linux",
		VersionLabel:   "Talos version",
		VersionDefault: "v1.10.5",
		RefLabel:       "Schematic ID",
		RefDefault:     vanillaSchematicID,
		RefHelp:        "Image Factory schematic ID from factory.talos.dev (default is the vanilla, no-extensions schematic).",
		ConfigFormat:   "yaml",
		Roles:          []string{models.ClusterRoleControlPlane, models.ClusterRoleWorker},
	}
}

func (Provider) ConfigTemplate(role string) string {
	roleType := "worker"
	if role == models.ClusterRoleControlPlane {
		roleType = "controlplane"
	}
	return fmt.Sprintf(`version: v1alpha1
machine:
  type: %s
  token: REPLACE_WITH_MACHINE_TOKEN
  ca:
    crt: REPLACE_WITH_CLUSTER_CA_CERT
  install:
    image: factory.talos.dev/installer/%s:v1.10.5
cluster:
  id: REPLACE_WITH_CLUSTER_ID
  secret: REPLACE_WITH_CLUSTER_SECRET
  controlPlane:
    endpoint: https://CONTROL_PLANE_ENDPOINT:6443
  token: REPLACE_WITH_CLUSTER_JOIN_TOKEN
  ca:
    crt: REPLACE_WITH_CLUSTER_CA_CERT
`, roleType, vanillaSchematicID)
}

func (Provider) ValidateConfig(config string) bool {
	docs := strings.SplitN(config, "\n---\n", 2)
	var doc struct {
		Version string `yaml:"version"`
		Machine *struct {
			Type string `yaml:"type"`
		} `yaml:"machine"`
	}
	if err := yaml.Unmarshal([]byte(docs[0]), &doc); err != nil {
		return false
	}
	return doc.Version == "v1alpha1" && doc.Machine != nil
}

func assetSubdir(image *models.NodeImage) string {
	return fmt.Sprintf("talos/%s/%s", image.Version, image.ImageRef)
}

func (Provider) Assets(image *models.NodeImage) ([]kubernetes.Asset, error) {
	if image.Version == "" || image.ImageRef == "" {
		return nil, fmt.Errorf("Talos node image %s needs a version and schematic ID", image.Name)
	}
	subdir := assetSubdir(image)
	return []kubernetes.Asset{
		{URL: fmt.Sprintf("%s/%s/%s/kernel-amd64", factoryBaseURL, image.ImageRef, image.Version), Path: subdir + "/vmlinuz"},
		{URL: fmt.Sprintf("%s/%s/%s/initramfs-amd64.xz", factoryBaseURL, image.ImageRef, image.Version), Path: subdir + "/initramfs.xz"},
	}, nil
}

func (Provider) InstallScript(image *models.NodeImage, cluster *models.Cluster, client *models.Client, ctx kubernetes.BootContext) (string, error) {
	assetBase := fmt.Sprintf("%s/talos/%s/%s", ctx.AssetBaseURL, url.PathEscape(image.Version), url.PathEscape(image.ImageRef))

	extraParams := strings.TrimSpace(cluster.KernelParams)
	if extraParams != "" {
		extraParams = " " + extraParams
	}

	var sb strings.Builder
	sb.WriteString("#!ipxe\n")
	sb.WriteString(fmt.Sprintf("echo Booting Talos %s installer for cluster %s (%s)...\n", image.Version, cluster.Name, kubernetes.Hostname(client)))
	sb.WriteString(fmt.Sprintf("kernel %s/vmlinuz initrd=initramfs.xz init_on_alloc=1 slab_nomerge pti=on console=tty0 console=ttyS0 printk.devkmsg=on talos.platform=metal talos.config=%s%s\n", assetBase, ctx.ConfigURL, extraParams))
	sb.WriteString(fmt.Sprintf("initrd %s/initramfs.xz\n", assetBase))
	sb.WriteString("boot\n")
	return sb.String(), nil
}

func (Provider) RenderConfig(cluster *models.Cluster, client *models.Client, serverAddr string) ([]byte, string, error) {
	body, err := render(cluster, client, serverAddr)
	if err != nil {
		return nil, "", err
	}
	return body, "application/yaml", nil
}

func render(cluster *models.Cluster, client *models.Client, serverAddr string) ([]byte, error) {
	var base string
	switch client.ClusterRole {
	case models.ClusterRoleControlPlane:
		base = cluster.ControlPlaneConfig
	case models.ClusterRoleWorker:
		base = cluster.WorkerConfig
	default:
		return nil, fmt.Errorf("client %s has unknown cluster role %q", client.MACAddress, client.ClusterRole)
	}
	if strings.TrimSpace(base) == "" {
		return nil, fmt.Errorf("cluster %s has no machine config for role %q", cluster.Name, client.ClusterRole)
	}

	vars := kubernetes.Vars(cluster, client, serverAddr)
	base = kubernetes.Substitute(base, vars)
	docs := strings.SplitN(base, "\n---\n", 2)

	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(docs[0]), &doc); err != nil {
		return nil, fmt.Errorf("machine config for cluster %s is not valid YAML: %w", cluster.Name, err)
	}
	if doc == nil {
		doc = map[string]interface{}{}
	}

	machineOverlay := map[string]interface{}{
		"network": map[string]interface{}{"hostname": kubernetes.Hostname(client)},
	}
	if disk := kubernetes.InstallDisk(cluster, client); disk != "" {
		machineOverlay["install"] = map[string]interface{}{"disk": disk}
	}
	doc = kubernetes.MergeYAML(doc, map[string]interface{}{"machine": machineOverlay})

	doc, err := kubernetes.ApplyPatch(doc, client.ConfigPatch, vars)
	if err != nil {
		return nil, fmt.Errorf("config patch for %s is not valid YAML: %w", client.MACAddress, err)
	}

	out, err := yaml.Marshal(doc)
	if err != nil {
		return nil, err
	}
	if len(docs) == 2 {
		out = append(out, []byte("---\n"+docs[1])...)
	}
	return out, nil
}
