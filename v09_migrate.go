package main

// migrateV09 fills 0.9 defaults. It never removes data or moves secrets.
func migrateV09(c *Config) {
	if c.Integrations == nil {
		c.Integrations = []*Integration{}
	}
	if c.Analytics == nil {
		c.Analytics = &AnalyticsSettings{Enabled: true, RetentionDays: 90, BotFilter: true}
	}
	if c.Alerts == nil {
		c.Alerts = &AlertSettings{InApp: true, Windows: true, CPU: 90, RAM: 90, Disk: 90, SSLDays: 21}
	}
	for _, s := range c.Sites {
		if s.Engine == "" {
			// Keep whatever the project's server already uses.
			if p := findServer(c, firstNonEmpty(s.Server, c.ActiveServer)); p != nil && p.Engine == "lite" {
				s.Engine = "nginx"
			}
		}
	}
}
