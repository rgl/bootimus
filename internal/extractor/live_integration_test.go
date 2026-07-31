package extractor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractLiveISO(t *testing.T) {
	isoPath := os.Getenv("BOOTIMUS_TEST_ISO")
	if isoPath == "" {
		t.Skip("BOOTIMUS_TEST_ISO not set")
	}

	dataDir := os.Getenv("BOOTIMUS_TEST_DATADIR")
	if dataDir == "" {
		dataDir = t.TempDir()
	}

	ext, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	ext.SetProgress(NewProgressReporter())

	files, err := ext.Extract(isoPath)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	t.Logf("Kernel: %s", files.Kernel)
	t.Logf("Initrd: %s", files.Initrd)
	t.Logf("Distro: %s", files.Distro)
	t.Logf("SquashfsPath: %s", files.SquashfsPath)
	t.Logf("NetbootRequired: %v", files.NetbootRequired)

	if files.NetbootRequired {
		t.Error("live ISO should not require netboot files")
	}

	if !strings.Contains(strings.ToLower(filepath.Base(isoPath)), "live") {
		return
	}

	if files.SquashfsPath == "" {
		t.Fatal("live ISO should have a squashfs path")
	}

	squashfsOnDisk := filepath.Join(dataDir, relativeISOBase(dataDir, isoPath), filepath.FromSlash(files.SquashfsPath))
	info, err := os.Stat(squashfsOnDisk)
	if err != nil {
		t.Fatalf("squashfs not on disk at %s: %v", squashfsOnDisk, err)
	}
	t.Logf("squashfs size on disk: %d", info.Size())

	kinfo, err := os.Stat(files.Kernel)
	if err != nil {
		t.Fatalf("kernel not on disk: %v", err)
	}
	if kinfo.Size() == 0 {
		t.Error("extracted kernel is empty")
	}
	iinfo, err := os.Stat(files.Initrd)
	if err != nil {
		t.Fatalf("initrd not on disk: %v", err)
	}
	if iinfo.Size() == 0 {
		t.Error("extracted initrd is empty")
	}
}
