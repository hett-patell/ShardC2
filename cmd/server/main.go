package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println(`
  ____  _                   _  ____ ____  
 / ___|| |__   __ _ _ __ __| |/ ___|___ \ 
 \___ \| '_ \ / _' | '__/ _' | |     __) |
  ___) | | | | (_| | | | (_| | |___ / __/ 
 |____/|_| |_|\__,_|_|  \__,_|\____|_____|
                                           
 Command & Control Framework v0.1.0
	`)
	fmt.Println("[*] ShardC2 Server starting...")
	fmt.Printf("[*] PID: %d\n", os.Getpid())
}
