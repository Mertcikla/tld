package main
import (
	"fmt"
	"path/filepath"
)
func main() {
	fmt.Printf("clean empty: %q\n", filepath.Clean(""))
	fmt.Printf("clean dot: %q\n", filepath.Clean("."))
}
