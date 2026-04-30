package server

// Server holds the ShardC2 C2 server state
type Server struct {
	Address string
}

// New creates a new ShardC2 server instance
func New(addr string) *Server {
	return &Server{Address: addr}
}
