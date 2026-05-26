package cmdbench

import (
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
)

// openBrowser launches the platform default browser on url. Best
// effort — failures are silent because the URL is already shown
// to the user in the terminal output.
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	}
	return nil
}

// waitForInterrupt blocks until the user sends SIGINT/SIGTERM.
// The HTTP server started by Run keeps the process alive; this
// is the cooperative stop point. Matches the cmdaudit idiom.
func waitForInterrupt() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
}
