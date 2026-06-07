package db

import "testing"

func TestMessageStamps(t *testing.T) {
	s := newTestStore(t)
	if err := s.gdb.Create(&Chat{ID: "c1", Source: "manual"}).Error; err != nil {
		t.Fatalf("create chat: %v", err)
	}
	msgs := []Message{
		{ID: "m1", ChatID: "c1", Role: "user", Content: "a", Timestamp: 100, Seq: 1},
		{ID: "m2", ChatID: "c1", Role: "assistant", Content: "b", Timestamp: 0, Seq: 2}, // sin timestamp → excluido
		{ID: "m3", ChatID: "c1", Role: "assistant", Content: "c", Timestamp: 200, Seq: 3},
	}
	if err := s.gdb.Create(&msgs).Error; err != nil {
		t.Fatalf("create messages: %v", err)
	}

	stamps, err := s.MessageStamps("c1")
	if err != nil {
		t.Fatalf("MessageStamps: %v", err)
	}
	if len(stamps) != 2 {
		t.Fatalf("got %d stamps, want 2 (timestamp>0 only)", len(stamps))
	}
	// Orden por Seq y roles correctos.
	if stamps[0].Role != "user" || stamps[0].Timestamp != 100 {
		t.Errorf("stamp[0] = %+v, want {user,100}", stamps[0])
	}
	if stamps[1].Role != "assistant" || stamps[1].Timestamp != 200 {
		t.Errorf("stamp[1] = %+v, want {assistant,200}", stamps[1])
	}
}
