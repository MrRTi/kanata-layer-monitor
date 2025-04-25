package main

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/getlantern/systray"
	"gopkg.in/yaml.v3"
)

type LayerChangeMsg struct {
	LayerChange struct {
		New string `json:"new"`
	} `json:"LayerChange"`
}

type LabelConfig struct {
	Text   *string `yaml:"text,omitempty"`
	Hidden bool    `yaml:"hidden"`
}

type IconConfig struct {
	Path *string `yaml:"path,omitempty"`
}

type LayerConfig struct {
	Label LabelConfig `yaml:"label"`
	Icon  IconConfig  `yaml:"icon"`
}

type Config struct {
	Host   string                 `yaml:"host"`
	Port   int                    `yaml:"port"`
	Layers map[string]LayerConfig `yaml:"layers"`
}

var (
	configPath   string
	config       Config
	configLock   sync.RWMutex
	currentLayer string
	iconsByLayer map[string][]byte
)

//go:embed icon.png
var defaultIcon []byte

func loadInitialConfig() {
	candidates := []string{
		filepath.Join(os.Getenv("HOME"), ".config", "kanata-layer-monitor", "config.yaml"),
		filepath.Join(os.Getenv("HOME"), ".config", "kanata-layer-monitor", "config.yml"),
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			configPath = path
			break
		}
	}

	if configPath != "" {
		if err := loadConfig(configPath); err != nil {
			log.Printf("Failed to load config: %v", err)
		}
	} else {
		log.Println("No config found, using defaults")
	}
}

func loadIcon(path string) (icon []byte) {
	iconPath := filepath.Join(filepath.Dir(configPath), path)
	icon, err := os.ReadFile(iconPath)

	if err != nil {
		log.Printf("Failed to load icon '%s': %v", iconPath, err)
	}

	return icon
}

func loadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	ext := strings.ToLower(filepath.Ext(path))

	if ext != ".yaml" && ext != ".yml" {
		log.Println("Config format not supported. Use yaml or yml. Using default configuration")
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return err
	}

	if config.Host == "" {
		config.Host = "127.0.0.1"
	}

	if config.Port == 0 {
		config.Port = 4444
	}

	log.Printf("Config loaded from %s", path)

	return nil
}

func loadIcons() {
	iconCache := make(map[string][]byte)

	for layer, layerConfig := range config.Layers {
		if layerConfig.Icon.Path != nil {
			if icon := loadIcon(*layerConfig.Icon.Path); icon != nil {
				iconCache[layer] = icon
				log.Printf("Icon for layer %s loaded", layer)
			}
		}
	}

	iconsByLayer = iconCache
}

func onReady() {
	systray.SetIcon(defaultIcon)
	changeShowedLayer("...")

	systray.SetTooltip("Kanata Layer Monitor")

	connectedInfo := fmt.Sprintf("Listening %s:%d", config.Host, config.Port)
	info := systray.AddMenuItem(connectedInfo, connectedInfo)
	info.Disable()

	systray.AddSeparator()
	quitItem := systray.AddMenuItem("Quit", "Quit the application")

	go quitChecker(*quitItem)
	go monitorLayer()
}

func onExit() {
	// Cleanup if needed
}

func quitChecker(quitItem systray.MenuItem) {
	<-quitItem.ClickedCh
	log.Printf("Shutting down")
	onExit()
	systray.Quit()
	os.Exit(0)
}

func monitorLayer() {
	for {
		conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", config.Host, config.Port))
		if err != nil {
			log.Printf("Connection failed: %v", err)
			changeShowedLayer("Error")
			return
		}

		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			line := scanner.Bytes()

			var msg LayerChangeMsg
			if err := json.Unmarshal(line, &msg); err != nil {
				log.Printf("Invalid JSON: %s", line)
				changeShowedLayer("N/A")
				continue
			}

			layer := msg.LayerChange.New
			if layer != currentLayer {
				currentLayer = layer
				changeShowedLayer(layer)
			}
		}
		conn.Close()
	}
}

func changeShowedLayer(layerName string) {
	configLock.RLock()
	defer configLock.RUnlock()

	layerConfig, ok := config.Layers[layerName]
	text := layerName
	icon := defaultIcon

	if ok {
		if layerConfig.Label.Text != nil {
			text = *layerConfig.Label.Text
		}
		if iconsByLayer[layerName] != nil {
			icon = iconsByLayer[layerName]
		}
	}

	if layerName != "N/A" && layerName != "Error" && layerConfig.Label.Hidden == true {
		text = ""
	}

	systray.SetTitle(fmt.Sprintf(" %s", text))
	systray.SetIcon(icon)

	writeToFile(text)

	log.Printf("Layer changed to \"%s\" (%s)", text, layerName)
}

func writeToFile(layer string) {
	dirPath := filepath.Join(os.Getenv("HOME"), ".cache", "kanata-layer-monitor")
	filePath := filepath.Join(dirPath, "current-layer")

	// Create the directories (if they don't exist)
	if err := os.MkdirAll(dirPath, os.ModePerm); err != nil {
		fmt.Println("Error creating directories:", err)
		return
	}

	// Open the file with O_RDWR|O_CREATE|O_TRUNC flags (rewrite)
	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0640)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	defer file.Close()

	_, err = file.WriteString(layer)
	if err != nil {
		fmt.Println("Error writing to file:", err)
		return
	}

}

func main() {
	loadInitialConfig()
	loadIcons()
	systray.Run(onReady, onExit)
}
