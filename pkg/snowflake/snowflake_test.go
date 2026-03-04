package snowflake

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestSnowflake_Generate(t *testing.T) {
	node, err := NewNode(1, customEpoch)
	if err != nil {
		t.Fatalf("failed to create node: %v", err)
	}

	ids := make(map[int64]bool)
	count := 10000

	for i := 0; i < count; i++ {
		id, err := node.Generate()
		if err != nil {
			t.Errorf("failed to generate ID: %v", err)
			continue
		}

		if ids[id] {
			t.Errorf("duplicate ID generated: %d", id)
		}
		ids[id] = true
	}

	if len(ids) != count {
		t.Errorf("expected %d unique IDs, got %d", count, len(ids))
	}
}

func TestSnowflake_GenerateString(t *testing.T) {
	idStr := GenerateString()

	// Verify it's a valid number string
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		t.Errorf("generated string is not a valid int64: %v", err)
	}

	if id <= 0 {
		t.Errorf("generated ID should be positive, got %d", id)
	}

	t.Logf("Generated ID: %s (length: %d)", idStr, len(idStr))
}

func TestSnowflake_GenerateStringWithPrefix(t *testing.T) {
	prefix := "task_"
	idStr := GenerateStringWithPrefix(prefix)

	if len(idStr) <= len(prefix) {
		t.Errorf("ID with prefix too short: %s", idStr)
	}

	// Extract and verify the numeric part
	numPart := idStr[len(prefix):]
	id, err := strconv.ParseInt(numPart, 10, 64)
	if err != nil {
		t.Errorf("numeric part is not valid: %v", err)
	}

	if id <= 0 {
		t.Errorf("generated ID should be positive, got %d", id)
	}

	t.Logf("Generated ID with prefix: %s", idStr)
}

func TestSnowflake_Uniqueness(t *testing.T) {
	node, _ := NewNode(1, customEpoch)

	const goroutines = 100
	const idsPerGoroutine = 1000

	var wg sync.WaitGroup
	ids := make(chan int64, goroutines*idsPerGoroutine)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < idsPerGoroutine; j++ {
				id, _ := node.Generate()
				ids <- id
			}
		}()
	}

	wg.Wait()
	close(ids)

	uniqueIDs := make(map[int64]bool)
	for id := range ids {
		if uniqueIDs[id] {
			t.Errorf("duplicate ID found: %d", id)
		}
		uniqueIDs[id] = true
	}

	expected := goroutines * idsPerGoroutine
	if len(uniqueIDs) != expected {
		t.Errorf("expected %d unique IDs, got %d", expected, len(uniqueIDs))
	}

	t.Logf("Generated %d unique IDs concurrently", len(uniqueIDs))
}

func TestSnowflake_MultiNode(t *testing.T) {
	node1, _ := NewNode(1, customEpoch)
	node2, _ := NewNode(2, customEpoch)

	id1, _ := node1.Generate()
	id2, _ := node2.Generate()

	nodeID1 := GetNodeID(id1)
	nodeID2 := GetNodeID(id2)

	if nodeID1 != 1 {
		t.Errorf("expected node ID 1, got %d", nodeID1)
	}
	if nodeID2 != 2 {
		t.Errorf("expected node ID 2, got %d", nodeID2)
	}

	t.Logf("Node 1 ID: %d (node: %d)", id1, nodeID1)
	t.Logf("Node 2 ID: %d (node: %d)", id2, nodeID2)
}

func TestSnowflake_ParseID(t *testing.T) {
	// Generate an ID
	node, _ := NewNode(123, customEpoch)
	id, _ := node.Generate()

	// Parse it
	timestamp, nodeID, sequence, err := ParseID(id)
	if err != nil {
		t.Fatalf("failed to parse ID: %v", err)
	}

	if nodeID != 123 {
		t.Errorf("expected node ID 123, got %d", nodeID)
	}

	if sequence < 0 || sequence > 4095 {
		t.Errorf("sequence out of range: %d", sequence)
	}

	// Verify timestamp is recent (within 1 second)
	now := time.Now()
	diff := now.Sub(timestamp)
	if diff < 0 {
		diff = -diff
	}
	if diff > time.Second {
		t.Errorf("timestamp differs too much from now: %v", diff)
	}

	t.Logf("Parsed ID: timestamp=%v, nodeID=%d, sequence=%d", timestamp, nodeID, sequence)
}

func TestSnowflake_ParseString(t *testing.T) {
	idStr := GenerateString()

	timestamp, nodeID, sequence, err := ParseString(idStr)
	if err != nil {
		t.Fatalf("failed to parse ID string: %v", err)
	}

	t.Logf("Parsed string ID %s: timestamp=%v, nodeID=%d, sequence=%d",
		idStr, timestamp, nodeID, sequence)
}

func TestSnowflake_GetTimestamp(t *testing.T) {
	before := time.Now()
	id := Generate()
	after := time.Now()

	ts := GetTimestamp(id)

	if ts.Before(before.Add(-time.Millisecond)) || ts.After(after.Add(time.Millisecond)) {
		t.Errorf("timestamp %v not between %v and %v", ts, before, after)
	}
}

func TestSnowflake_Ordering(t *testing.T) {
	node, _ := NewNode(1, customEpoch)

	var prevID int64 = 0
	for i := 0; i < 1000; i++ {
		id, _ := node.Generate()
		if id <= prevID {
			t.Errorf("IDs not increasing: prev=%d, current=%d", prevID, id)
		}
		prevID = id
	}
}

func TestSnowflake_InvalidNodeID(t *testing.T) {
	_, err := NewNode(-1, customEpoch)
	if err == nil {
		t.Error("expected error for negative node ID")
	}

	_, err = NewNode(1024, customEpoch)
	if err == nil {
		t.Error("expected error for node ID > 1023")
	}
}

func TestSnowflake_Init(t *testing.T) {
	err := Init(&Config{NodeID: 100})
	if err != nil {
		t.Fatalf("failed to init: %v", err)
	}

	id := Generate()
	nodeID := GetNodeID(id)

	if nodeID != 100 {
		t.Errorf("expected node ID 100, got %d", nodeID)
	}
}

func BenchmarkSnowflake_Generate(b *testing.B) {
	node, _ := NewNode(1, customEpoch)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = node.Generate()
	}
}

func BenchmarkSnowflake_GenerateString(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GenerateString()
	}
}
