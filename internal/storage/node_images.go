package storage

import (
	"fmt"

	"bootimus/internal/models"

	"gorm.io/gorm"
)

func listNodeImages(db *gorm.DB) ([]*models.NodeImage, error) {
	var images []*models.NodeImage
	if err := db.Order("name").Find(&images).Error; err != nil {
		return nil, err
	}
	return images, nil
}

func getNodeImage(db *gorm.DB, id uint) (*models.NodeImage, error) {
	var image models.NodeImage
	if err := db.First(&image, id).Error; err != nil {
		return nil, err
	}
	return &image, nil
}

func createNodeImage(db *gorm.DB, image *models.NodeImage) error {
	return db.Create(image).Error
}

func setNodeImageDownloaded(db *gorm.DB, id uint, downloaded bool) error {
	return db.Model(&models.NodeImage{}).Where("id = ?", id).Update("downloaded", downloaded).Error
}

func deleteNodeImage(db *gorm.DB, id uint) error {
	var count int64
	if err := db.Model(&models.Cluster{}).Where("node_image_id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("node image is in use by %d cluster(s)", count)
	}
	result := db.Unscoped().Delete(&models.NodeImage{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *SQLiteStore) ListNodeImages() ([]*models.NodeImage, error) { return listNodeImages(s.db) }
func (s *SQLiteStore) GetNodeImage(id uint) (*models.NodeImage, error) {
	return getNodeImage(s.db, id)
}
func (s *SQLiteStore) CreateNodeImage(image *models.NodeImage) error {
	return createNodeImage(s.db, image)
}
func (s *SQLiteStore) SetNodeImageDownloaded(id uint, downloaded bool) error {
	return setNodeImageDownloaded(s.db, id, downloaded)
}
func (s *SQLiteStore) DeleteNodeImage(id uint) error { return deleteNodeImage(s.db, id) }

func (s *PostgresStore) ListNodeImages() ([]*models.NodeImage, error) { return listNodeImages(s.db) }
func (s *PostgresStore) GetNodeImage(id uint) (*models.NodeImage, error) {
	return getNodeImage(s.db, id)
}
func (s *PostgresStore) CreateNodeImage(image *models.NodeImage) error {
	return createNodeImage(s.db, image)
}
func (s *PostgresStore) SetNodeImageDownloaded(id uint, downloaded bool) error {
	return setNodeImageDownloaded(s.db, id, downloaded)
}
func (s *PostgresStore) DeleteNodeImage(id uint) error { return deleteNodeImage(s.db, id) }
