// Package snowflake implements the Twitter Snowflake ID generation algorithm.
// Generates 64-bit, unique, time-sortable IDs in a distributed environment.
//
// ID Structure (64 bits):
// 0 - 0000000000 0000000000 0000000000 0000000000 0 - 0000000000 - 000000000000
// |   --------------------41 bits--------------------   ---10 bits---   ---12 bits---
// |                          |                              |                 |
// |                          |                              |                 └── Sequence (0-4095)
// |                          |                              └── Node ID (0-1023)
// |                          └── Timestamp (milliseconds since custom epoch)
// └── Sign bit (always 0)
package snowflake

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"
)

const (
	// Bit lengths
	nodeBits  = 10 // Node ID bits (0-1023 nodes)
	seqBits   = 12 // Sequence bits (0-4095 per ms)

	// Masks
	nodeMax   = (1 << nodeBits) - 1 // 1023
	seqMax    = (1 << seqBits) - 1  // 4095

	// Shifts
	nodeShift = seqBits                   // 12
	timeShift = seqBits + nodeBits        // 22

	// Custom epoch (2024-01-01 00:00:00 UTC)
	// This gives us ~69 years of IDs
	customEpoch = 1704067200000
)

var (
	// ErrInvalidNodeID is returned when node ID is out of range
	ErrInvalidNodeID = errors.New("node ID must be between 0 and 1023")

	// ErrClockMovedBack is returned when system clock moved backwards
	ErrClockMovedBack = errors.New("clock moved backwards, refusing to generate ID")
)

// Node represents a snowflake ID generator node
type Node struct {
	mu        sync.Mutex
	nodeID    int64
	lastTime  int64
	sequence  int64
	epoch     int64
}

// Global default node instance
var defaultNode *Node
var once sync.Once

// Config holds the configuration for snowflake ID generation
type Config struct {
	// NodeID is the unique identifier for this node (0-1023)
	NodeID int64
	// Epoch is the custom epoch in milliseconds (default: 2024-01-01)
	Epoch int64
}

// Init initializes the default snowflake node with the given configuration
func Init(cfg *Config) error {
	if cfg == nil {
		cfg = &Config{}
	}

	nodeID := cfg.NodeID
	if nodeID < 0 || nodeID > nodeMax {
		return ErrInvalidNodeID
	}

	epoch := cfg.Epoch
	if epoch == 0 {
		epoch = customEpoch
	}

	var err error
	defaultNode, err = NewNode(nodeID, epoch)
	return err
}

// NewNode creates a new snowflake ID generator node
func NewNode(nodeID, epoch int64) (*Node, error) {
	if nodeID < 0 || nodeID > nodeMax {
		return nil, ErrInvalidNodeID
	}

	return &Node{
		nodeID: nodeID,
		epoch:  epoch,
	}, nil
}

// Generate generates a new unique snowflake ID (int64)
func (n *Node) Generate() (int64, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	now := time.Now().UnixMilli() - n.epoch

	if now < n.lastTime {
		return 0, fmt.Errorf("%w: last=%d, now=%d", ErrClockMovedBack, n.lastTime, now)
	}

	if now == n.lastTime {
		n.sequence = (n.sequence + 1) & seqMax
		if n.sequence == 0 {
			// Sequence overflow, wait for next millisecond
			for now <= n.lastTime {
				time.Sleep(100 * time.Microsecond)
				now = time.Now().UnixMilli() - n.epoch
			}
		}
	} else {
		n.sequence = 0
	}

	n.lastTime = now

	id := (now << timeShift) | (n.nodeID << nodeShift) | n.sequence
	return id, nil
}

// Generate generates a new unique snowflake ID using the default node
func Generate() int64 {
	once.Do(func() {
		if defaultNode == nil {
			// Initialize with node ID 0 if not configured
			defaultNode, _ = NewNode(0, customEpoch)
		}
	})

	id, err := defaultNode.Generate()
	if err != nil {
		panic(err) // Should never happen in normal operation
	}
	return id
}

// GenerateString generates a new unique snowflake ID as a string
func GenerateString() string {
	return strconv.FormatInt(Generate(), 10)
}

// GenerateStringWithPrefix generates a new unique snowflake ID with a prefix
func GenerateStringWithPrefix(prefix string) string {
	return prefix + GenerateString()
}

// ParseID parses a snowflake ID string and returns its components
func ParseID(id int64) (timestamp time.Time, nodeID int, sequence int, err error) {
	if id < 0 {
		err = errors.New("invalid snowflake ID: must be positive")
		return
	}

	// Extract components
	seq := id & seqMax
	node := (id >> nodeShift) & nodeMax
	ms := (id >> timeShift) + customEpoch

	timestamp = time.UnixMilli(ms)
	nodeID = int(node)
	sequence = int(seq)

	return
}

// ParseString parses a snowflake ID string and returns its components
func ParseString(idStr string) (timestamp time.Time, nodeID int, sequence int, err error) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		err = fmt.Errorf("invalid snowflake ID string: %w", err)
		return
	}
	return ParseID(id)
}

// GetTimestamp extracts the timestamp from a snowflake ID
func GetTimestamp(id int64) time.Time {
	ms := (id >> timeShift) + customEpoch
	return time.UnixMilli(ms)
}

// GetNodeID extracts the node ID from a snowflake ID
func GetNodeID(id int64) int {
	return int((id >> nodeShift) & nodeMax)
}

// GetSequence extracts the sequence from a snowflake ID
func GetSequence(id int64) int {
	return int(id & seqMax)
}

// Now returns the current timestamp in milliseconds since epoch
func Now() int64 {
	return time.Now().UnixMilli() - customEpoch
}
