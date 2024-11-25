// SPDX-License-Identifier: GPL-3.0-or-later

package proxysql

type (
	cache struct {
		commands map[string]*commandCache
		users    map[string]*userCache
		backends map[string]*backendCache
	}
	commandCache struct {
		command            string
		hasCharts, updated bool
	}
	userCache struct {
		user               string
		hasCharts, updated bool
	}
	backendCache struct {
		hg, host, port     string
		hasCharts, updated bool
	}
)

func (c *cache) reset() {
	for k, m := range c.commands {
		c.commands[k] = &commandCache{command: m.command, hasCharts: m.hasCharts}
	}
	for k, m := range c.users {
		c.users[k] = &userCache{user: m.user, hasCharts: m.hasCharts}
	}
	for k, m := range c.backends {
		c.backends[k] = &backendCache{hg: m.hg, host: m.host, port: m.port, hasCharts: m.hasCharts}
	}
}

func (c *cache) getCommand(command string) *commandCache {
	v, ok := c.commands[command]
	if !ok {
		v = &commandCache{command: command}
		c.commands[command] = v
	}
	return v
}

func (c *cache) getUser(user string) *userCache {
	v, ok := c.users[user]
	if !ok {
		v = &userCache{user: user}
		c.users[user] = v
	}
	return v
}

func (c *cache) getBackend(hg, host, port string) *backendCache {
	id := backendID(hg, host, port)
	v, ok := c.backends[id]
	if !ok {
		v = &backendCache{hg: hg, host: host, port: port}
		c.backends[id] = v
	}
	return v
}
