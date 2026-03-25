package comutation

// Config holds server configuration. Methods that update Config
// should write Host, Port, and Timeout together (co-mutation belief).
type Config struct {
	Host    string
	Port    int
	Timeout int
}

// Reset writes all three fields — co-mutated.
func (c *Config) Reset(host string, port int, timeout int) {
	c.Host = host
	c.Port = port
	c.Timeout = timeout
}

// Init writes all three fields — co-mutated.
func (c *Config) Init(host string, port int, timeout int) {
	c.Host = host
	c.Port = port
	c.Timeout = timeout
}

// Update writes all three fields — co-mutated.
func (c *Config) Update(host string, port int, timeout int) {
	c.Host = host
	c.Port = port
	c.Timeout = timeout
}

// Restore writes all three fields — co-mutated.
func (c *Config) Restore(host string, port int, timeout int) {
	c.Host = host
	c.Port = port
	c.Timeout = timeout
}

// SetServer writes only Host and Port — intentional deviation.
func (c *Config) SetServer(host string, port int) {
	c.Host = host
	c.Port = port
}
