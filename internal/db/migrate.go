package db

import (
	"fmt"

	"gorm.io/gorm"
)

// ftsSchema crea la tabla virtual FTS5 sobre el contenido de los mensajes y los
// triggers que la mantienen sincronizada automáticamente. Al ser standalone
// (no external-content) guardamos message_id sin indexar para mapear los
// resultados de búsqueda de vuelta a la tabla messages.
//
// El tokenizer porter da stemming en inglés; unicode61 normaliza acentos para
// que "decay" y "Decáy" matcheen. BM25 viene incluido en FTS5.
const ftsSchema = `
CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
    message_id UNINDEXED,
    content,
    tokenize = 'porter unicode61'
);

CREATE TRIGGER IF NOT EXISTS messages_fts_ai AFTER INSERT ON messages BEGIN
    INSERT INTO messages_fts(message_id, content) VALUES (new.id, new.content);
END;

CREATE TRIGGER IF NOT EXISTS messages_fts_ad AFTER DELETE ON messages BEGIN
    DELETE FROM messages_fts WHERE message_id = old.id;
END;

CREATE TRIGGER IF NOT EXISTS messages_fts_au AFTER UPDATE ON messages BEGIN
    UPDATE messages_fts SET content = new.content WHERE message_id = old.id;
END;
`

// migrate corre AutoMigrate sobre los modelos relacionales y luego crea la capa
// FTS5 en SQL crudo. Es idempotente: seguro de llamar en cada apertura.
func migrate(gdb *gorm.DB) error {
	if err := gdb.AutoMigrate(models()...); err != nil {
		return fmt.Errorf("failed to auto-migrate models: %w", err)
	}
	if err := gdb.Exec(ftsSchema).Error; err != nil {
		return fmt.Errorf("failed to create FTS5 schema: %w", err)
	}
	return nil
}
