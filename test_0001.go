package main

import (
	"fmt"
	"sync"
)

func main() {
	var once sync.Once

	for i := 0; i < 5; i++ {
		once.Do(func() {
			fmt.Println("This will print only once.")
		})
	}
}
