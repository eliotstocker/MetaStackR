package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Exact ASCII logo matching the website (web/template.html)
var LogoLines = []string{
	`_  _ ____ ___ ____ ____ ___ ____ ____ _  _ ____`,
	`|\/| |___  |  |__| [__   |  |__| |    |_/  |__/`,
	`|  | |___  |  |  | ___]  |  |  | |___ | \_ |  \ `,
}

// Colors matching website linear-gradient(90deg, #00FF88, #FACC15)
const (
	StartR, StartG, StartB = 0, 255, 136  // #00FF88 (Neon Emerald)
	EndR, EndG, EndB       = 250, 204, 21 // #FACC15 (Bright Yellow)
)

// GetBanner returns the colored ASCII logo string with a horizontal gradient matching the website
func GetBanner() string {
	var maxLen int
	for _, line := range LogoLines {
		if len(line) > maxLen {
			maxLen = len(line)
		}
	}
	if maxLen == 0 {
		maxLen = 1
	}

	var b strings.Builder
	b.WriteString("\n")

	for _, line := range LogoLines {
		runes := []rune(line)
		for i, char := range runes {
			t := float64(i) / float64(maxLen-1)
			if t > 1.0 {
				t = 1.0
			}

			r := int(float64(StartR)*(1.0-t) + float64(EndR)*t)
			g := int(float64(StartG)*(1.0-t) + float64(EndG)*t)
			bCol := int(float64(StartB)*(1.0-t) + float64(EndB)*t)

			hex := fmt.Sprintf("#%02X%02X%02X", r, g, bCol)
			style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(hex))
			b.WriteString(style.Render(string(char)))
		}
		b.WriteString("\n")
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
