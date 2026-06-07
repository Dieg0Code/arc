package db

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UpsertChat inserta el chat o, si ya existe (mismo id), actualiza su metadata
// editable (título, source, ruta de sesión). created_at se preserva del primero.
func (s *store) UpsertChat(chat *Chat) error {
	if chat == nil || chat.ID == "" {
		return errors.New("chat with non-empty id is required")
	}
	err := s.gdb.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"title", "source", "session_path"}),
	}).Create(chat).Error
	if err != nil {
		return fmt.Errorf("failed to upsert chat %s: %w", chat.ID, err)
	}
	return nil
}

// GetChat devuelve un chat por id, o (nil, nil) si no existe.
func (s *store) GetChat(id string) (*Chat, error) {
	var chat Chat
	err := s.gdb.First(&chat, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get chat %s: %w", id, err)
	}
	return &chat, nil
}

// ListChats devuelve todos los chats (id, título, source, etc.), ordenados por
// fecha de creación. Se usa para resolver scopes de acceso.
func (s *store) ListChats() ([]Chat, error) {
	var chats []Chat
	if err := s.gdb.Order("created_at ASC").Find(&chats).Error; err != nil {
		return nil, fmt.Errorf("failed to list chats: %w", err)
	}
	return chats, nil
}

// CountMessages cuenta los mensajes de un chat.
func (s *store) CountMessages(chatID string) (int64, error) {
	var n int64
	err := s.gdb.Model(&Message{}).Where("chat_id = ?", chatID).Count(&n).Error
	if err != nil {
		return 0, fmt.Errorf("failed to count messages for chat %s: %w", chatID, err)
	}
	return n, nil
}
