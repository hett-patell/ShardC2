package bruteforcer

// Bruteforcer coordinates SSH brute force attacks
type Bruteforcer struct {
	Target  string
	Port    int
	Workers int
}

// New creates a new bruteforcer instance
func New(target string, port int, workers int) *Bruteforcer {
	return &Bruteforcer{
		Target:  target,
		Port:    port,
		Workers: workers,
	}
}
