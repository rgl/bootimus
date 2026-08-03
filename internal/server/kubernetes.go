package server

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"bootimus/internal/kubernetes"
	"bootimus/internal/models"

	_ "bootimus/internal/kubernetes/flatcar"
	_ "bootimus/internal/kubernetes/talos"
	"bootimus/internal/webhook"
)

func (s *Server) provisionAssetDir() string {
	return filepath.Join(s.config.DataDir, "node-images")
}

func (s *Server) provisionBootContext(client *models.Client) kubernetes.BootContext {
	base := fmt.Sprintf("http://%s:%d", s.config.ServerAddr, s.config.HTTPPort)
	return kubernetes.BootContext{
		AssetBaseURL: base + "/node-images",
		ConfigURL:    fmt.Sprintf("%s/machineconfig?m=${net0/mac}&u=${uuid}&t=%s", base, client.EnrollmentToken),
	}
}

func (s *Server) provisionInstallScriptFor(client *models.Client) (string, bool) {
	if client == nil || s.config.Storage == nil || client.ClusterID == nil {
		return "", false
	}
	if err := kubernetes.ValidateToken(client, client.EnrollmentToken, time.Now()); err != nil {
		log.Printf("Provision boot for %s skipped: %v", client.MACAddress, err)
		return "", false
	}
	cluster, err := s.config.Storage.GetCluster(*client.ClusterID)
	if err != nil {
		log.Printf("Provision boot for %s skipped: cluster lookup failed: %v", client.MACAddress, err)
		return "", false
	}
	if cluster.NodeImageID == nil {
		log.Printf("Provision boot for %s skipped: cluster %s has no node image", client.MACAddress, cluster.Name)
		return "", false
	}
	nodeImage, err := s.config.Storage.GetNodeImage(*cluster.NodeImageID)
	if err != nil {
		log.Printf("Provision boot for %s skipped: node image lookup failed: %v", client.MACAddress, err)
		return "", false
	}
	if !nodeImage.Downloaded {
		log.Printf("Provision boot for %s skipped: node image %s not downloaded", client.MACAddress, nodeImage.Name)
		return "", false
	}
	provider, err := kubernetes.GetProvider(nodeImage.Provider)
	if err != nil {
		log.Printf("Provision boot for %s skipped: %v", client.MACAddress, err)
		return "", false
	}
	script, err := provider.InstallScript(nodeImage, cluster, client, s.provisionBootContext(client))
	if err != nil {
		log.Printf("Provision boot for %s skipped: %v", client.MACAddress, err)
		return "", false
	}
	return script, true
}

func (s *Server) handleMachineConfig(w http.ResponseWriter, r *http.Request) {
	if s.config.Storage == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	mac := strings.ToLower(strings.ReplaceAll(r.URL.Query().Get("m"), "-", ":"))
	token := r.URL.Query().Get("t")
	uuid := r.URL.Query().Get("u")

	deny := func(reason string) {
		log.Printf("machineconfig: DENIED request for mac=%q from %s: %s", mac, r.RemoteAddr, reason)
		http.Error(w, "Forbidden", http.StatusForbidden)
	}

	client, err := s.config.Storage.GetClient(mac)
	if err != nil {
		deny("unknown client")
		return
	}
	if err := kubernetes.ValidateToken(client, token, time.Now()); err != nil {
		deny(err.Error())
		return
	}
	if inv, err := s.config.Storage.GetLatestHardwareInventory(mac); err == nil && inv != nil {
		if err := kubernetes.ValidateUUID(inv.UUID, uuid); err != nil {
			deny(fmt.Sprintf("%v (expected inventory uuid, got %q)", err, uuid))
			return
		}
	}
	if client.ClusterID == nil {
		deny("client has no cluster assignment")
		return
	}
	cluster, err := s.config.Storage.GetCluster(*client.ClusterID)
	if err != nil {
		deny("cluster not found")
		return
	}
	if cluster.NodeImageID == nil {
		deny("cluster has no node image")
		return
	}
	nodeImage, err := s.config.Storage.GetNodeImage(*cluster.NodeImageID)
	if err != nil {
		deny("cluster node image not found")
		return
	}
	provider, err := kubernetes.GetProvider(nodeImage.Provider)
	if err != nil {
		deny(err.Error())
		return
	}

	rendered, contentType, err := provider.RenderConfig(cluster, client, s.config.ServerAddr)
	if err != nil {
		log.Printf("machineconfig: render failed for %s: %v", mac, err)
		http.Error(w, "Machine config render failed", http.StatusInternalServerError)
		return
	}

	if client.EnrollmentState == models.EnrollmentStateApproved {
		if err := s.config.Storage.SetClientEnrollmentState(mac, models.EnrollmentStateInstalling); err != nil {
			log.Printf("machineconfig: failed to mark %s installing: %v", mac, err)
		} else {
			s.webhookNotifier.Fire(webhook.Event{
				Event:      webhook.EventInstallStarted,
				MAC:        mac,
				ClientName: client.Name,
				IP:         r.RemoteAddr,
				Metadata:   map[string]string{"cluster": cluster.Name, "role": client.ClusterRole},
			})
		}
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(rendered)))
	w.Write(rendered)
	s.logAndBroadcast("Served machine config to %s (cluster: %s, role: %s, provider: %s)", mac, cluster.Name, client.ClusterRole, provider.Name())
}
