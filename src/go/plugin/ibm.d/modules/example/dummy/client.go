package dummy

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// Item represents an item returned by the dummy protocol
type Item struct {
	Slot       int
	Percentage float64
}

// Client simulates a protocol client that would normally connect to a real system
type Client struct {
	mu              sync.Mutex
	connected       bool
	activeSlots     map[int]bool
	nextSlot        int
	collectionCount int

	// Simulated persistent state (would normally be on the server)
	serverCounter int64

	// Cached static data (refreshed on each connection)
	cachedVersion string
	cachedEdition string
}

// NewClient creates a new dummy protocol client
func NewClient() *Client {
	c := &Client{
		activeSlots: make(map[int]bool),
		nextSlot:    10,
	}

	// Initialize with 10 slots
	for i := 0; i < 10; i++ {
		c.activeSlots[i] = true
	}

	return c
}

// GetVersion returns cached version information (refreshed on each connection)
func (c *Client) GetVersion() (string, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return "", "", fmt.Errorf("not connected")
	}

	// Return cached version data - this is refreshed only on connection
	return c.cachedVersion, c.cachedEdition, nil
}

// refreshStaticData simulates fetching static data from the server on connection
func (c *Client) refreshStaticData() {
	// Simulate version information that might change between connections
	c.cachedVersion = "2.4.1"
	c.cachedEdition = "Community"

	// Simulate different editions based on connection time
	if time.Now().Unix()%3 == 0 {
		c.cachedEdition = "Enterprise"
	}
}

// Connect simulates establishing a connection
func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return fmt.Errorf("already connected")
	}

	// Simulate connection delay
	time.Sleep(10 * time.Millisecond)

	c.connected = true

	// Refresh static data on every connection
	c.refreshStaticData()

	return nil
}

// Disconnect simulates closing the connection
func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return fmt.Errorf("not connected")
	}

	c.connected = false
	return nil
}

// IsConnected returns true if connected
func (c *Client) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

// GetTimestampValue simulates fetching a single value from the protocol
// In a real protocol, this would be a network call
func (c *Client) GetTimestampValue() (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return 0, fmt.Errorf("not connected")
	}

	// Simulate protocol response: timestamp % 60
	now := time.Now().Unix()
	value := now % 60

	// Update server-side counter (simulating server state)
	c.serverCounter += value

	return value, nil
}

// GetCounterValue returns the current counter value from the "server"
func (c *Client) GetCounterValue() (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return 0, fmt.Errorf("not connected")
	}

	return c.serverCounter, nil
}

// ListItems simulates fetching a list of items with percentages
// Every 5th call, it removes one item and adds a new one
// maxItems limits the number of items returned
func (c *Client) ListItems(maxItems int) ([]Item, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil, fmt.Errorf("not connected")
	}

	c.collectionCount++

	// Every 5th collection, rotate items
	if c.collectionCount%5 == 0 && c.collectionCount > 0 {
		c.rotateItems()
	}

	// Generate items with random percentages that sum to 100
	items := c.generateItems(maxItems)

	return items, nil
}

// rotateItems removes one slot and adds a new one
func (c *Client) rotateItems() {
	// Find a random slot to remove
	var slotsToRemove []int
	for slot := range c.activeSlots {
		slotsToRemove = append(slotsToRemove, slot)
	}

	if len(slotsToRemove) > 0 {
		// Pick a random slot to remove
		removeIdx := rand.Intn(len(slotsToRemove))
		slotToRemove := slotsToRemove[removeIdx]

		// Remove the slot
		delete(c.activeSlots, slotToRemove)

		// Add a new slot
		c.activeSlots[c.nextSlot] = true
		c.nextSlot++
	}
}

// generateItems creates items with random percentages totaling 100%
func (c *Client) generateItems(maxItems int) []Item {
	items := make([]Item, 0, len(c.activeSlots))
	remaining := 100.0

	// Convert active slots to sorted slice for consistent ordering
	var slots []int
	for slot := range c.activeSlots {
		slots = append(slots, slot)
	}

	// Sort slots
	for i := 0; i < len(slots)-1; i++ {
		for j := i + 1; j < len(slots); j++ {
			if slots[i] > slots[j] {
				slots[i], slots[j] = slots[j], slots[i]
			}
		}
	}

	// Limit to maxItems if specified
	if maxItems > 0 && len(slots) > maxItems {
		slots = slots[:maxItems]
	}

	// Generate random percentages
	for i, slot := range slots {
		var value float64
		if i < len(slots)-1 {
			// Random value from remaining
			value = rand.Float64() * remaining
			remaining -= value
		} else {
			// Last item gets the remaining
			value = remaining
		}

		items = append(items, Item{
			Slot:       slot,
			Percentage: value,
		})
	}

	return items
}
