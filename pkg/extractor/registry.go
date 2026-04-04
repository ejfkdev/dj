package extractor

// PluginRegistry 插件注册中心
type PluginRegistry struct {
	plugins []Plugin
}

// NewPluginRegistry 创建插件注册中心
func NewPluginRegistry() *PluginRegistry {
	return &PluginRegistry{
		plugins: make([]Plugin, 0),
	}
}

// Register 注册插件
func (r *PluginRegistry) Register(p Plugin) {
	r.plugins = append(r.plugins, p)
}

// GetAll 获取所有插件
func (r *PluginRegistry) GetAll() []Plugin {
	return r.plugins
}
