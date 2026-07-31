package storage

import (
	"testing"

	"bootimus/internal/models"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	store, err := NewSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	if err := store.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return store
}

func TestDeleteClientHardDeletesAndAllowsRecreate(t *testing.T) {
	store := newTestStore(t)
	mac := "aa:bb:cc:dd:ee:ff"

	if err := store.CreateClient(&models.Client{MACAddress: mac, Name: "first"}); err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	if err := store.DeleteClient(mac); err != nil {
		t.Fatalf("DeleteClient: %v", err)
	}

	var count int64
	store.db.Unscoped().Model(&models.Client{}).Where("mac_address = ?", mac).Count(&count)
	if count != 0 {
		t.Fatalf("expected client row to be gone, found %d rows", count)
	}

	if err := store.CreateClient(&models.Client{MACAddress: mac, Name: "second"}); err != nil {
		t.Fatalf("CreateClient after delete: %v", err)
	}
	client, err := store.GetClient(mac)
	if err != nil {
		t.Fatalf("GetClient after recreate: %v", err)
	}
	if client.Name != "second" {
		t.Fatalf("expected recreated client, got %q", client.Name)
	}
}

func TestDeleteClientMissingMACIsNoop(t *testing.T) {
	store := newTestStore(t)
	if err := store.DeleteClient("00:00:00:00:00:00"); err != nil {
		t.Fatalf("DeleteClient on missing MAC: %v", err)
	}
}

func TestDeleteClientDetachesBootLogsAndInventory(t *testing.T) {
	store := newTestStore(t)
	mac := "aa:bb:cc:dd:ee:01"

	if err := store.CreateClient(&models.Client{MACAddress: mac}); err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	client, err := store.GetClient(mac)
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if err := store.db.Create(&models.BootLog{ClientID: &client.ID, MACAddress: mac}).Error; err != nil {
		t.Fatalf("create boot log: %v", err)
	}
	if err := store.db.Create(&models.HardwareInventory{ClientID: &client.ID, MACAddress: mac}).Error; err != nil {
		t.Fatalf("create inventory: %v", err)
	}

	if err := store.DeleteClient(mac); err != nil {
		t.Fatalf("DeleteClient: %v", err)
	}

	var bootLog models.BootLog
	if err := store.db.Where("mac_address = ?", mac).First(&bootLog).Error; err != nil {
		t.Fatalf("boot log should survive client deletion: %v", err)
	}
	if bootLog.ClientID != nil {
		t.Fatalf("expected boot log client_id to be nulled, got %d", *bootLog.ClientID)
	}

	var inv models.HardwareInventory
	if err := store.db.Where("mac_address = ?", mac).First(&inv).Error; err != nil {
		t.Fatalf("inventory should survive client deletion: %v", err)
	}
	if inv.ClientID != nil {
		t.Fatalf("expected inventory client_id to be nulled, got %d", *inv.ClientID)
	}
}

func TestAutoMigratePurgesLegacySoftDeletedClients(t *testing.T) {
	store := newTestStore(t)
	mac := "aa:bb:cc:dd:ee:02"

	if err := store.CreateClient(&models.Client{MACAddress: mac}); err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	if err := store.db.Where("mac_address = ?", mac).Delete(&models.Client{}).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	var count int64
	store.db.Unscoped().Model(&models.Client{}).Where("mac_address = ?", mac).Count(&count)
	if count != 1 {
		t.Fatalf("expected soft-deleted row to exist before migration, found %d", count)
	}

	if err := store.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	store.db.Unscoped().Model(&models.Client{}).Where("mac_address = ?", mac).Count(&count)
	if count != 0 {
		t.Fatalf("expected soft-deleted row to be purged, found %d", count)
	}

	if err := store.CreateClient(&models.Client{MACAddress: mac}); err != nil {
		t.Fatalf("CreateClient after purge: %v", err)
	}
}
