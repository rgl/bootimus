package storage

import (
	"time"

	"bootimus/internal/models"

	"gorm.io/gorm"
)

func listClusters(db *gorm.DB) ([]*models.Cluster, error) {
	var clusters []*models.Cluster
	if err := db.Order("name").Find(&clusters).Error; err != nil {
		return nil, err
	}
	return clusters, nil
}

func getCluster(db *gorm.DB, id uint) (*models.Cluster, error) {
	var cluster models.Cluster
	if err := db.First(&cluster, id).Error; err != nil {
		return nil, err
	}
	return &cluster, nil
}

func createCluster(db *gorm.DB, cluster *models.Cluster) error {
	return db.Create(cluster).Error
}

func updateCluster(db *gorm.DB, id uint, cluster *models.Cluster) error {
	result := db.Model(&models.Cluster{}).Where("id = ?", id).
		Select("Name", "Description", "NodeImageID", "KernelParams", "DefaultInstallDisk", "UpdatedAt").
		Updates(cluster)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func setClusterConfigs(db *gorm.DB, id uint, controlPlane, worker, source string) error {
	updates := map[string]interface{}{}
	if controlPlane != "" {
		updates["control_plane_config"] = controlPlane
	}
	if worker != "" {
		updates["worker_config"] = worker
	}
	if source != "" {
		updates["config_source"] = source
	}
	if len(updates) == 0 {
		return nil
	}
	result := db.Model(&models.Cluster{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func deleteCluster(db *gorm.DB, id uint) error {
	if err := db.Model(&models.Client{}).Where("cluster_id = ?", id).Updates(map[string]interface{}{
		"cluster_id":       nil,
		"enrollment_state": models.EnrollmentStateUnmanaged,
		"cluster_role":     "",
		"install_disk":     "",
		"config_patch":     "",
		"enrollment_token": "",
		"token_expires_at": nil,
		"approved_at":      nil,
	}).Error; err != nil {
		return err
	}
	return db.Unscoped().Delete(&models.Cluster{}, id).Error
}

func listClusterNodes(db *gorm.DB, clusterID uint) ([]*models.Client, error) {
	var clients []*models.Client
	if err := db.Where("cluster_id = ?", clusterID).Order("name").Find(&clients).Error; err != nil {
		return nil, err
	}
	return clients, nil
}

func assignNodeToCluster(db *gorm.DB, mac string, clusterID uint, role, installDisk, configPatch, token string, expires time.Time) error {
	now := time.Now()
	result := db.Model(&models.Client{}).Where("mac_address = ?", mac).Updates(map[string]interface{}{
		"enrollment_state": models.EnrollmentStateApproved,
		"cluster_id":       clusterID,
		"cluster_role":     role,
		"install_disk":     installDisk,
		"config_patch":     configPatch,
		"enrollment_token": token,
		"token_expires_at": expires,
		"approved_at":      now,
		"static":           true,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func setClientEnrollmentState(db *gorm.DB, mac, state string) error {
	updates := map[string]interface{}{"enrollment_state": state}
	switch state {
	case models.EnrollmentStateInstalled, models.EnrollmentStateRejected, models.EnrollmentStateUnmanaged:
		updates["enrollment_token"] = ""
		updates["token_expires_at"] = nil
	}
	result := db.Model(&models.Client{}).Where("mac_address = ?", mac).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func resetClientEnrollment(db *gorm.DB, mac string) error {
	result := db.Model(&models.Client{}).Where("mac_address = ?", mac).Updates(map[string]interface{}{
		"enrollment_state": models.EnrollmentStateUnmanaged,
		"cluster_id":       nil,
		"cluster_role":     "",
		"install_disk":     "",
		"config_patch":     "",
		"enrollment_token": "",
		"token_expires_at": nil,
		"approved_at":      nil,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *SQLiteStore) ListClusters() ([]*models.Cluster, error) { return listClusters(s.db) }
func (s *SQLiteStore) GetCluster(id uint) (*models.Cluster, error) {
	return getCluster(s.db, id)
}
func (s *SQLiteStore) CreateCluster(cluster *models.Cluster) error {
	return createCluster(s.db, cluster)
}
func (s *SQLiteStore) UpdateCluster(id uint, cluster *models.Cluster) error {
	return updateCluster(s.db, id, cluster)
}
func (s *SQLiteStore) SetClusterConfigs(id uint, controlPlane, worker, source string) error {
	return setClusterConfigs(s.db, id, controlPlane, worker, source)
}
func (s *SQLiteStore) DeleteCluster(id uint) error { return deleteCluster(s.db, id) }
func (s *SQLiteStore) ListClusterNodes(clusterID uint) ([]*models.Client, error) {
	return listClusterNodes(s.db, clusterID)
}
func (s *SQLiteStore) AssignNodeToCluster(mac string, clusterID uint, role, installDisk, configPatch, token string, expires time.Time) error {
	return assignNodeToCluster(s.db, mac, clusterID, role, installDisk, configPatch, token, expires)
}
func (s *SQLiteStore) SetClientEnrollmentState(mac, state string) error {
	return setClientEnrollmentState(s.db, mac, state)
}
func (s *SQLiteStore) ResetClientEnrollment(mac string) error {
	return resetClientEnrollment(s.db, mac)
}

func (s *PostgresStore) ListClusters() ([]*models.Cluster, error) { return listClusters(s.db) }
func (s *PostgresStore) GetCluster(id uint) (*models.Cluster, error) {
	return getCluster(s.db, id)
}
func (s *PostgresStore) CreateCluster(cluster *models.Cluster) error {
	return createCluster(s.db, cluster)
}
func (s *PostgresStore) UpdateCluster(id uint, cluster *models.Cluster) error {
	return updateCluster(s.db, id, cluster)
}
func (s *PostgresStore) SetClusterConfigs(id uint, controlPlane, worker, source string) error {
	return setClusterConfigs(s.db, id, controlPlane, worker, source)
}
func (s *PostgresStore) DeleteCluster(id uint) error { return deleteCluster(s.db, id) }
func (s *PostgresStore) ListClusterNodes(clusterID uint) ([]*models.Client, error) {
	return listClusterNodes(s.db, clusterID)
}
func (s *PostgresStore) AssignNodeToCluster(mac string, clusterID uint, role, installDisk, configPatch, token string, expires time.Time) error {
	return assignNodeToCluster(s.db, mac, clusterID, role, installDisk, configPatch, token, expires)
}
func (s *PostgresStore) SetClientEnrollmentState(mac, state string) error {
	return setClientEnrollmentState(s.db, mac, state)
}
func (s *PostgresStore) ResetClientEnrollment(mac string) error {
	return resetClientEnrollment(s.db, mac)
}
