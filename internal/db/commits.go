package db

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// CreateCommit persiste un commit inmutable.
func (s *store) CreateCommit(c *Commit) error {
	if c == nil || c.Hash == "" || c.ChatID == "" {
		return errors.New("commit with hash and chatID is required")
	}
	if err := s.gdb.Create(c).Error; err != nil {
		return fmt.Errorf("failed to create commit %s: %w", c.Hash, err)
	}
	return nil
}

// HeadCommit devuelve el commit más reciente del chat, o (nil, nil) si no hay.
func (s *store) HeadCommit(chatID string) (*Commit, error) {
	var c Commit
	err := s.gdb.
		Where("chat_id = ?", chatID).
		Order("created_at DESC").
		First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get head commit for chat %s: %w", chatID, err)
	}
	return &c, nil
}

// GetCommit devuelve un commit por hash. Acepta un prefijo siempre que sea
// único; si el prefijo es ambiguo devuelve error.
func (s *store) GetCommit(hash string) (*Commit, error) {
	if hash == "" {
		return nil, errors.New("hash is required")
	}
	var commits []Commit
	err := s.gdb.Where("hash = ? OR hash LIKE ?", hash, hash+"%").Limit(2).Find(&commits).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get commit %s: %w", hash, err)
	}
	switch len(commits) {
	case 0:
		return nil, nil
	case 1:
		return &commits[0], nil
	default:
		// Si hay match exacto entre los dos, preferirlo.
		for i := range commits {
			if commits[i].Hash == hash {
				return &commits[i], nil
			}
		}
		return nil, fmt.Errorf("commit prefix %q is ambiguous", hash)
	}
}

// ListCommits lista los commits del chat, del más nuevo al más viejo.
func (s *store) ListCommits(chatID string) ([]Commit, error) {
	var commits []Commit
	err := s.gdb.
		Where("chat_id = ?", chatID).
		Order("created_at DESC").
		Find(&commits).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list commits for chat %s: %w", chatID, err)
	}
	return commits, nil
}

// ListAllCommits lista todos los commits (para exportar en sync).
func (s *store) ListAllCommits() ([]Commit, error) {
	var commits []Commit
	err := s.gdb.Order("created_at ASC").Find(&commits).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list all commits: %w", err)
	}
	return commits, nil
}
