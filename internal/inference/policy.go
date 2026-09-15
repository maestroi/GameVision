package inference

// EnsureMaxTokens raises the completion budget when a caller needs a richer
// response contract. It never lowers an explicitly larger configured budget.
func (c *Client) EnsureMaxTokens(min int) {
	if min <= 0 {
		return
	}
	c.mu.Lock()
	if c.cfg.MaxTokens < min {
		c.cfg.MaxTokens = min
	}
	c.mu.Unlock()
}
