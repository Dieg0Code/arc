package db

// Chat representa una conversación ingestada desde un agente (codex, claude) o
// creada manualmente. Es el contenedor de mensajes y commits.
type Chat struct {
	ID          string `gorm:"primaryKey"`
	Title       string
	Source      string `gorm:"index"` // "codex" | "claude" | "manual"
	CreatedAt   int64  // unix seconds, autocompletado por GORM
	SessionPath string

	Messages []Message `gorm:"foreignKey:ChatID;constraint:OnDelete:CASCADE"`
}

// Message es un turno individual dentro de un chat. El campo Seq da orden
// determinístico dentro del chat (los timestamps pueden colisionar).
type Message struct {
	ID         string `gorm:"primaryKey"`
	ChatID     string `gorm:"index"`
	Role       string // "user" | "assistant" | "tool" | "system"
	Content    string
	Timestamp  int64
	TokenCount int
	Seq        int64 `gorm:"index"` // orden dentro del chat
}

// Commit es un snapshot INMUTABLE de un rango de mensajes. Copia el texto en
// Snapshot (JSON) al momento de commitear, de modo que reingestar o editar
// mensajes nunca altera lo que un commit ya capturó: "tu agente olvida, arc no".
type Commit struct {
	Hash      string `gorm:"primaryKey"`
	ChatID    string `gorm:"index"`
	Branch    string `gorm:"default:main"`
	Message   string // mensaje del commit, escrito por el agente o el humano
	MsgFrom   string // id del primer mensaje del rango
	MsgTo     string // id del último mensaje del rango
	Snapshot  string // JSON con el texto copiado de los mensajes (inmutable)
	CreatedAt int64
}

// Staging es el index git-like: los mensajes marcados con `arc add` que esperan
// ser commiteados. Una fila por mensaje staged del chat activo.
type Staging struct {
	ID        string `gorm:"primaryKey"`
	ChatID    string `gorm:"index;uniqueIndex:idx_staging_chat_msg"`
	MsgID     string `gorm:"uniqueIndex:idx_staging_chat_msg"`
	Seq       int64  // copia del Seq del mensaje para ordenar el rango
	CreatedAt int64
}

// Memory es la capa MUTABLE sobre el registro inmutable: un hecho/decisión
// destilado que el agente lee al empezar una sesión. Puede actualizarse, y
// referencia el commit que lo respalda como evidencia.
type Memory struct {
	ID         string `gorm:"primaryKey"`
	ChatID     string `gorm:"index"`
	Content    string
	CommitHash string // commit que respalda este recuerdo (evidencia)
	CreatedAt  int64
	UpdatedAt  int64
}

// models devuelve todos los modelos para AutoMigrate.
func models() []any {
	return []any{
		&Chat{},
		&Message{},
		&Commit{},
		&Staging{},
		&Memory{},
	}
}
