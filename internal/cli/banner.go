package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var LogoLines = []string{
	`  __  __      _         ____  _             _        ____  `,
	` |  \/  | ___| |_ __ _ / ___|| |_ __ _  ___| | _____|  _ \ `,
	` | |\/| |/ _ \ __/ _' |\___ \| __/ _' |/ __| |/ / _ \ |_) |`,
	` | |  | |  __/ || (_| | ___) | || (_| | (__|   <  __/  _ < `,
	` |_|  |_|\___|\__\__,_||____/ \__\__,_|\___|_|\_\___|_| \_\`,
}

// Gradient colors transitioning from vibrant purple through cyan to emerald
var GradientColors = []string{
	"#A855F7", // Purple
	"#8B5CF6", // Violet
	"#6366F1", // Indigo
	"#3B82F6", // Blue
	"#06B6D4", // Cyan
	"#10B981", // Emerald
}

// GetBanner returns the colored ASCII logo string with a smooth vertical gradient
func GetBanner() string {
	var b strings.Builder
	b.WriteString("\n")
	for i, line := range LogoLines {
		colorIdx := i % len(GradientColors)
		style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(GradientColors[colorIdx]))
		b.WriteString(style.Render(line) + "\n")
	}
	b.WriteString("\n")
	return b.String()
}

// PrintBanner prints the gradient ASCII logo unless --json is enabled
func PrintBanner() {
	if !jsonOutput {
		fmt.Print(GetBanner())
	}
}
