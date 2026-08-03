package kubernetes

import (
	"fmt"
	"sort"
	"strings"

	"bootimus/internal/models"

	yaml "go.yaml.in/yaml/v3"
)

type Asset struct {
	URL  string
	Path string
}

type BootContext struct {
	AssetBaseURL string
	ConfigURL    string
}

type ProviderInfo struct {
	Name           string   `json:"name"`
	Label          string   `json:"label"`
	VersionLabel   string   `json:"version_label"`
	VersionDefault string   `json:"version_default"`
	RefLabel       string   `json:"ref_label"`
	RefDefault     string   `json:"ref_default"`
	RefHelp        string   `json:"ref_help"`
	ConfigFormat   string   `json:"config_format"`
	Roles          []string `json:"roles"`
}

type Provider interface {
	Name() string
	Info() ProviderInfo
	ConfigTemplate(role string) string
	ValidateConfig(config string) bool
	Assets(image *models.NodeImage) ([]Asset, error)
	InstallScript(image *models.NodeImage, cluster *models.Cluster, client *models.Client, ctx BootContext) (string, error)
	RenderConfig(cluster *models.Cluster, client *models.Client, serverAddr string) (body []byte, contentType string, err error)
}

var providers = map[string]Provider{}

func RegisterProvider(p Provider) {
	providers[p.Name()] = p
}

func GetProvider(name string) (Provider, error) {
	p, ok := providers[name]
	if !ok {
		return nil, fmt.Errorf("unknown provisioning provider %q", name)
	}
	return p, nil
}

func ProviderNames() []string {
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func ProvidersInfo() []ProviderInfo {
	infos := make([]ProviderInfo, 0, len(providers))
	for _, name := range ProviderNames() {
		infos = append(infos, providers[name].Info())
	}
	return infos
}

func Hostname(client *models.Client) string {
	if client.Name != "" {
		return client.Name
	}
	suffix := strings.ReplaceAll(client.MACAddress, ":", "")
	if len(suffix) > 6 {
		suffix = suffix[len(suffix)-6:]
	}
	return "node-" + suffix
}

func InstallDisk(cluster *models.Cluster, client *models.Client) string {
	if client.InstallDisk != "" {
		return client.InstallDisk
	}
	return cluster.DefaultInstallDisk
}

func Vars(cluster *models.Cluster, client *models.Client, serverAddr string) map[string]string {
	return map[string]string{
		"{{MAC}}":          client.MACAddress,
		"{{HOSTNAME}}":     Hostname(client),
		"{{SERVER_ADDR}}":  serverAddr,
		"{{INSTALL_DISK}}": InstallDisk(cluster, client),
		"{{CLUSTER_NAME}}": cluster.Name,
	}
}

func Substitute(s string, vars map[string]string) string {
	for k, v := range vars {
		s = strings.ReplaceAll(s, k, v)
	}
	return s
}

func MergeYAML(dst, src map[string]interface{}) map[string]interface{} {
	if dst == nil {
		return src
	}
	for k, v := range src {
		if sv, ok := v.(map[string]interface{}); ok {
			if dv, ok := dst[k].(map[string]interface{}); ok {
				dst[k] = MergeYAML(dv, sv)
				continue
			}
		}
		dst[k] = v
	}
	return dst
}

func ApplyPatch(doc map[string]interface{}, patch string, vars map[string]string) (map[string]interface{}, error) {
	if strings.TrimSpace(patch) == "" {
		return doc, nil
	}
	var parsed map[string]interface{}
	if err := yaml.Unmarshal([]byte(Substitute(patch, vars)), &parsed); err != nil {
		return nil, err
	}
	return MergeYAML(doc, parsed), nil
}
