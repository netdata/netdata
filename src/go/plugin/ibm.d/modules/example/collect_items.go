package example

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/example/contexts"
)

// collectItemMetrics collects metrics for all items
func (c *Collector) collectItemMetrics() error {
	// Fetch items from protocol with configured max items
	items, err := c.client.ListItems(c.config.MaxItems)
	if err != nil {
		return err
	}

	// Track which slots we've seen
	seenSlots := make(map[int]bool)

	// Process items
	for _, item := range items {
		seenSlots[item.Slot] = true

		// Create labels for this item
		labels := contexts.ItemLabels{
			Slot: fmt.Sprintf("%d", item.Slot),
		}

		// Set the absolute percentage value using type-safe API
		contexts.Item.ItemPercentage.Set(c.State, labels, contexts.ItemItemPercentageValues{
			Percentage: int64(item.Percentage * 100), // Convert to percentage integer
		})

		// Get or create persistent data for this item
		data := c.itemData[item.Slot]
		if data == nil {
			data = &ItemData{Counter: 0}
			c.itemData[item.Slot] = data
		}

		// Increment the item counter by the percentage value
		data.Counter += int64(item.Percentage * 100)

		// Set the incremental counter using type-safe API
		contexts.Item.ItemCounter.Set(c.State, labels, contexts.ItemItemCounterValues{
			Counter: data.Counter,
		})
	}

	// Clean up data for slots that no longer exist
	for slot := range c.itemData {
		if !seenSlots[slot] {
			delete(c.itemData, slot)
			c.Debugf("Removed data for slot %d", slot)
		}
	}

	c.Debugf("Collected %d items from protocol", len(items))

	return nil
}
