package db

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ClearNodes borra todo el árbol de índice. Los triggers de nodes_fts limpian
// el índice de búsqueda. Se usa para un rebuild completo de `nem index`.
func (s *store) ClearNodes() error {
	// Where("1 = 1") para borrar todas las filas (GORM exige condición).
	if err := s.gdb.Where("1 = 1").Delete(&Node{}).Error; err != nil {
		return fmt.Errorf("failed to clear nodes: %w", err)
	}
	return nil
}

// UpsertNodes inserta o actualiza nodos por id, en lotes. Devuelve cuántas filas
// se afectaron. Los triggers mantienen nodes_fts sincronizado.
func (s *store) UpsertNodes(nodes []Node) (int64, error) {
	if len(nodes) == 0 {
		return 0, nil
	}
	tx := s.gdb.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"parent_id", "kind", "chat_id", "title", "summary",
			"msg_from_seq", "msg_to_seq", "commit_hash", "created_at",
			"tokens", "superseded", "superseded_by", "pinned",
			"active_secs", "wall_secs", "sessions", "last_active",
		}),
	}).CreateInBatches(nodes, insertBatchSize)
	if tx.Error != nil {
		return 0, fmt.Errorf("failed to upsert nodes: %w", tx.Error)
	}
	return tx.RowsAffected, nil
}

// GetNode devuelve un nodo por id, o (nil, nil) si no existe.
func (s *store) GetNode(id string) (*Node, error) {
	var n Node
	err := s.gdb.First(&n, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get node %s: %w", id, err)
	}
	return &n, nil
}

// SetNodeSummary reescribe el resumen de un nodo y lo marca como Pinned (escrito
// por el agente/humano), de modo que un re-index no lo vuelva a generar. Es la
// capa mutable: el agente corrige resúmenes flojos o equivocados. Error si el
// nodo no existe.
func (s *store) SetNodeSummary(id, summary string) error {
	res := s.gdb.Model(&Node{}).Where("id = ?", id).
		Updates(map[string]any{"summary": summary, "pinned": true})
	if res.Error != nil {
		return fmt.Errorf("failed to set summary for node %s: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("node %s not found", id)
	}
	return nil
}

// PinnedNodes devuelve los nodos con resumen escrito a mano (Pinned), para que
// Build los respete en vez de regenerarlos.
func (s *store) PinnedNodes() ([]Node, error) {
	var nodes []Node
	if err := s.gdb.Where("pinned = ?", true).Find(&nodes).Error; err != nil {
		return nil, fmt.Errorf("failed to load pinned nodes: %w", err)
	}
	return nodes, nil
}

// ChildNodes devuelve los hijos directos de un nodo, ordenados por CreatedAt.
func (s *store) ChildNodes(parentID string) ([]Node, error) {
	var nodes []Node
	err := s.gdb.Where("parent_id = ?", parentID).
		Order("created_at ASC, title ASC").
		Find(&nodes).Error
	if err != nil {
		return nil, fmt.Errorf("failed to load children of %s: %w", parentID, err)
	}
	return nodes, nil
}

// RootNodes devuelve los nodos raíz (sin padre).
func (s *store) RootNodes() ([]Node, error) {
	var nodes []Node
	err := s.gdb.Where("parent_id = ?", "").Order("title ASC").Find(&nodes).Error
	if err != nil {
		return nil, fmt.Errorf("failed to load root nodes: %w", err)
	}
	return nodes, nil
}

// CountNodes cuenta los nodos del árbol.
func (s *store) CountNodes() (int64, error) {
	var n int64
	if err := s.gdb.Model(&Node{}).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("failed to count nodes: %w", err)
	}
	return n, nil
}

// SearchNodes corre BM25 sobre title+summary de los nodos. chatIDs limita por
// scope (vacío/nil = sin filtro).
func (s *store) SearchNodes(query string, limit int, chatIDs []string) ([]NodeHit, error) {
	if limit <= 0 {
		limit = 10
	}
	args := []any{query}
	chatClause := ""
	if len(chatIDs) > 0 {
		chatClause = "AND n.chat_id IN (?)"
		args = append(args, chatIDs)
	}
	args = append(args, limit)

	sql := fmt.Sprintf(`
		SELECT n.*, bm25(nodes_fts) AS score
		FROM nodes_fts
		JOIN nodes n ON n.id = nodes_fts.node_id
		WHERE nodes_fts MATCH ? %s
		ORDER BY score
		LIMIT ?`, chatClause)

	var hits []NodeHit
	if err := s.gdb.Raw(sql, args...).Scan(&hits).Error; err != nil {
		return nil, fmt.Errorf("failed to search nodes: %w", err)
	}
	return hits, nil
}

// CommitNodes devuelve los nodos de commit de los chats dados, ordenados por
// CreatedAt ascendente (vista temporal). chatIDs vacío/nil = todos los chats.
func (s *store) CommitNodes(chatIDs []string) ([]Node, error) {
	q := s.gdb.Where("kind = ?", "commit")
	if len(chatIDs) > 0 {
		q = q.Where("chat_id IN ?", chatIDs)
	}
	var nodes []Node
	if err := q.Order("created_at ASC").Find(&nodes).Error; err != nil {
		return nil, fmt.Errorf("failed to load commit nodes: %w", err)
	}
	return nodes, nil
}
