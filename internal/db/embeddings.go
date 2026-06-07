package db

import (
	"fmt"

	"gorm.io/gorm/clause"
)

// ClearEmbeddings borra todos los vectores (para un rebuild de embeddings).
func (s *store) ClearEmbeddings() error {
	if err := s.gdb.Where("1 = 1").Delete(&Embedding{}).Error; err != nil {
		return fmt.Errorf("failed to clear embeddings: %w", err)
	}
	return nil
}

// UpsertEmbeddings inserta o actualiza vectores por NodeID, en lotes.
func (s *store) UpsertEmbeddings(embs []Embedding) (int64, error) {
	if len(embs) == 0 {
		return 0, nil
	}
	tx := s.gdb.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "node_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"dim", "vec"}),
	}).CreateInBatches(embs, insertBatchSize)
	if tx.Error != nil {
		return 0, fmt.Errorf("failed to upsert embeddings: %w", tx.Error)
	}
	return tx.RowsAffected, nil
}

// AllEmbeddings devuelve todos los vectores guardados.
func (s *store) AllEmbeddings() ([]Embedding, error) {
	var embs []Embedding
	if err := s.gdb.Find(&embs).Error; err != nil {
		return nil, fmt.Errorf("failed to load embeddings: %w", err)
	}
	return embs, nil
}
