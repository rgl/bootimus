package flatcar

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"bootimus/internal/kubernetes"
	"bootimus/internal/models"
)

type Provider struct{}

func init() {
	kubernetes.RegisterProvider(Provider{})
}

func (Provider) Name() string {
	return models.ProviderFlatcar
}

func (Provider) Info() kubernetes.ProviderInfo {
	return kubernetes.ProviderInfo{
		Name:           models.ProviderFlatcar,
		Label:          "Flatcar Container Linux",
		VersionLabel:   "Flatcar version",
		VersionDefault: "current",
		RefLabel:       "Release channel",
		RefDefault:     "stable",
		RefHelp:        "Flatcar release channel (stable, beta, or alpha). Use version 'current' for the channel's latest release.",
		ConfigFormat:   "json",
		Roles:          []string{models.ClusterRoleControlPlane, models.ClusterRoleWorker},
	}
}

func (Provider) ConfigTemplate(role string) string {
	return `{
  "ignition": {"version": "3.4.0"},
  "storage": {
    "files": [
      {"path": "/etc/hostname", "mode": 420, "contents": {"source": "data:,{{HOSTNAME}}"}}
    ]
  },
  "systemd": {
    "units": []
  }
}
`
}

func (Provider) ValidateConfig(config string) bool {
	var doc struct {
		Ignition *struct {
			Version string `json:"version"`
		} `json:"ignition"`
	}
	if err := json.Unmarshal([]byte(config), &doc); err != nil {
		return false
	}
	return doc.Ignition != nil && doc.Ignition.Version != ""
}

func channelVersion(image *models.NodeImage) (string, string) {
	channel := image.ImageRef
	if channel == "" {
		channel = "stable"
	}
	version := image.Version
	if version == "" {
		version = "current"
	}
	return channel, version
}

func assetSubdir(image *models.NodeImage) string {
	channel, version := channelVersion(image)
	return fmt.Sprintf("flatcar/%s/%s", version, channel)
}

func (Provider) Assets(image *models.NodeImage) ([]kubernetes.Asset, error) {
	channel, version := channelVersion(image)
	base := fmt.Sprintf("https://%s.release.flatcar-linux.net/amd64-usr/%s", channel, version)
	subdir := assetSubdir(image)
	return []kubernetes.Asset{
		{URL: base + "/flatcar_production_pxe.vmlinuz", Path: subdir + "/vmlinuz"},
		{URL: base + "/flatcar_production_pxe_image.cpio.gz", Path: subdir + "/initrd.cpio.gz"},
	}, nil
}

func (Provider) InstallScript(image *models.NodeImage, cluster *models.Cluster, client *models.Client, ctx kubernetes.BootContext) (string, error) {
	channel, version := channelVersion(image)
	assetBase := fmt.Sprintf("%s/flatcar/%s/%s", ctx.AssetBaseURL, url.PathEscape(version), url.PathEscape(channel))

	extraParams := strings.TrimSpace(cluster.KernelParams)
	if extraParams != "" {
		extraParams = " " + extraParams
	}

	var sb strings.Builder
	sb.WriteString("#!ipxe\n")
	sb.WriteString(fmt.Sprintf("echo Booting Flatcar %s/%s for cluster %s (%s)...\n", channel, version, cluster.Name, kubernetes.Hostname(client)))
	sb.WriteString(fmt.Sprintf("kernel %s/vmlinuz initrd=initrd.cpio.gz flatcar.first_boot=1 ignition.config.url=%s console=tty0 console=ttyS0%s\n", assetBase, ctx.ConfigURL, extraParams))
	sb.WriteString(fmt.Sprintf("initrd %s/initrd.cpio.gz\n", assetBase))
	sb.WriteString("boot\n")
	return sb.String(), nil
}

func (Provider) RenderConfig(cluster *models.Cluster, client *models.Client, serverAddr string) ([]byte, string, error) {
	var base string
	switch client.ClusterRole {
	case models.ClusterRoleControlPlane:
		base = cluster.ControlPlaneConfig
	case models.ClusterRoleWorker:
		base = cluster.WorkerConfig
	default:
		return nil, "", fmt.Errorf("client %s has unknown cluster role %q", client.MACAddress, client.ClusterRole)
	}
	if strings.TrimSpace(base) == "" {
		return nil, "", fmt.Errorf("cluster %s has no join config for role %q", cluster.Name, client.ClusterRole)
	}

	vars := kubernetes.Vars(cluster, client, serverAddr)
	base = kubernetes.Substitute(base, vars)

	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(base), &doc); err != nil {
		return nil, "", fmt.Errorf("ignition config for cluster %s is not valid JSON: %w", cluster.Name, err)
	}

	doc, err := kubernetes.ApplyPatch(doc, client.ConfigPatch, vars)
	if err != nil {
		return nil, "", fmt.Errorf("config patch for %s is not valid YAML: %w", client.MACAddress, err)
	}

	out, err := json.Marshal(doc)
	if err != nil {
		return nil, "", err
	}
	return out, "application/json", nil
}
