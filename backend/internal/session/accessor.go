package session

func (m *Manager) CookieName() string {
	return m.config.Name
}
