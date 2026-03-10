package ui

import (
	"fmt"
	"time"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner displays an animated spinner with a message until the returned stop function is called.
// The stop function clears the spinner line before returning.
func Spinner(message string) (stop func()) {
	done := make(chan struct{})
	stopped := make(chan struct{})

	go func() {
		defer close(stopped)
		i := 0
		tick := time.NewTicker(80 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-done:
				fmt.Print("\r\033[K") // clear spinner line
				return
			case <-tick.C:
				fmt.Printf("\r  \033[2m%s\033[0m  %s", spinnerFrames[i%len(spinnerFrames)], message)
				i++
			}
		}
	}()

	return func() {
		close(done)
		<-stopped // wait for goroutine to clear the line
	}
}
