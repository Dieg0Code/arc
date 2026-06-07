package timing

import "github.com/Dieg0Code/nem/internal/db"

// SpanForChats computa el Span agregado de uno o varios chats leyendo sus
// timestamps del store. Reusado por `nem timeline`, `nem stats`, el tool MCP
// nem_duration y el `index` (para poblar la duración en los nodos).
func SpanForChats(store db.Store, chatIDs []string) (Span, error) {
	var total Span
	for _, id := range chatIDs {
		stamps, err := store.MessageStamps(id)
		if err != nil {
			return Span{}, err
		}
		evs := make([]Event, len(stamps))
		for i, s := range stamps {
			evs[i] = Event{Role: s.Role, Ts: s.Timestamp}
		}
		total = total.Merge(Compute(evs))
	}
	return total, nil
}
