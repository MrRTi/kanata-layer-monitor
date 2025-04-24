package main

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/getlantern/systray"
)

type LayerChangeMsg struct {
	LayerChange struct {
		New string `json:"new"`
	} `json:"LayerChange"`
}

//go:embed icon.png
var iconData []byte

var currentLayer string

func main() {
	systray.Run(onReady, onExit)
}

func onReady() {
	systray.SetTemplateIcon(iconData, iconData)
	systray.SetTitle("...")
	systray.SetTooltip("Kanata Layer Monitor")
	quitItem := systray.AddMenuItem("Quit", "Quit the application")

	go quitChecker(*quitItem)
	go monitorLayer()
}

// Listen for menu item click and exit when Quit is clicked
func quitChecker(quitItem systray.MenuItem) {
	<-quitItem.ClickedCh
	log.Printf("Shutting down")
	onExit()
	systray.Quit()
	os.Exit(0)
}

func onExit() {
	// Cleanup if needed
}

func monitorLayer() {
	conn, err := net.Dial("tcp", "127.0.0.1:4444")
	if err != nil {
		log.Printf("Failed to connect: %v", err)
		systray.SetTitle("Error")
		return
	}
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Bytes()

		var msg LayerChangeMsg
		if err := json.Unmarshal(line, &msg); err != nil {
			log.Printf("Invalid JSON: %s\nError: %v\n", line, err)
			continue
		}

		layer := msg.LayerChange.New
		if layer != currentLayer {
			currentLayer = layer
			title := fmt.Sprintf("%s", currentLayer)
			systray.SetTitle(title)
			log.Printf("Kanata layer changed to %s", title)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Scanner error: %v", err)
		systray.SetTitle("N/A")
		time.Sleep(5 * time.Second)
		os.Exit(1)
	}
}
