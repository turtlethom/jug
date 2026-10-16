// Command pathpin is a bookmark-first interactive directory navigator.
package main

import "github.com/turtlethom/pathpin/cmd"

// main only hands off to cmd: package main can't be imported by tests, so
// everything worth testing lives in cmd and internal/.
func main() {
	cmd.Execute()
}
