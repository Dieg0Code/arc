package db

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm/clause"
)

// StageMessages agrega los mensajes dados al staging del chat. Es idempotente
// por (chat, mensaje): re-stagear el mismo mensaje no duplica. Devuelve cuántas
// filas nuevas quedaron staged.
func (s *store) StageMessages(chatID string, msgs []Message) (int64, error) {
	if chatID == "" {
		return 0, fmt.Errorf("chatID is required")
	}
	if len(msgs) == 0 {
		return 0, nil
	}
	now := time.Now().Unix()
	rows := make([]Staging, 0, len(msgs))
	for _, m := range msgs {
		rows = append(rows, Staging{
			ID:        uuid.NewString(),
			ChatID:    chatID,
			MsgID:     m.ID,
			Seq:       m.Seq,
			CreatedAt: now,
		})
	}
	// Evitar duplicar el mismo mensaje en staging: ignorar si (chat,msg) ya está.
	tx := s.gdb.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "chat_id"}, {Name: "msg_id"}},
		DoNothing: true,
	}).Create(&rows)
	if tx.Error != nil {
		return 0, fmt.Errorf("failed to stage messages: %w", tx.Error)
	}
	return tx.RowsAffected, nil
}

// StagedMessages devuelve los mensajes staged del chat, ordenados por Seq.
func (s *store) StagedMessages(chatID string) ([]Message, error) {
	var msgs []Message
	err := s.gdb.Raw(`
		SELECT m.* FROM messages m
		JOIN stagings st ON st.msg_id = m.id AND st.chat_id = m.chat_id
		WHERE st.chat_id = ?
		ORDER BY m.seq ASC`, chatID).Scan(&msgs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to load staged messages for chat %s: %w", chatID, err)
	}
	return msgs, nil
}

// CountStaged cuenta los mensajes staged del chat.
func (s *store) CountStaged(chatID string) (int64, error) {
	var n int64
	err := s.gdb.Model(&Staging{}).Where("chat_id = ?", chatID).Count(&n).Error
	if err != nil {
		return 0, fmt.Errorf("failed to count staged for chat %s: %w", chatID, err)
	}
	return n, nil
}

// ClearStaging vacía el staging del chat.
func (s *store) ClearStaging(chatID string) error {
	err := s.gdb.Where("chat_id = ?", chatID).Delete(&Staging{}).Error
	if err != nil {
		return fmt.Errorf("failed to clear staging for chat %s: %w", chatID, err)
	}
	return nil
}
