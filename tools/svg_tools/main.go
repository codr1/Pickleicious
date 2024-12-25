package main

import (
	"fmt"
	"os"
	"regexp"
	"sort"

	"github.com/lucasb-eyer/go-colorful" // For color manipulation
)

// AttributeUsage tracks the usage of a specific color
// It includes the attribute name, the color value, and the count of occurrences
type AttributeUsage struct {
	Attribute string
	Color     string
	Count     int
}

// ColorMatch represents a color with its human-readable name and distance
// Distance measures how similar the input color is to a predefined color
type ColorMatch struct {
	Name     string
	ColorHex string
	Distance float64
}

// Predefined colors for matching
// These represent commonly recognized colors with their corresponding hex values
var humanReadableColors = map[string]string{
	"Black":     "#000000",
	"White":     "#FFFFFF",
	"Red":       "#FF0000",
	"Green":     "#008000",
	"Blue":      "#0000FF",
	"Yellow":    "#FFFF00",
	"Cyan":      "#00FFFF",
	"Magenta":   "#FF00FF",
	"Gray":      "#808080",
	"Silver":    "#C0C0C0",
	"Maroon":    "#800000",
	"Olive":     "#808000",
	"Lime":      "#00FF00",
	"Teal":      "#008080",
	"Navy":      "#000080",
	"Purple":    "#800080",
	"Orange":    "#FFA500",
	"Pink":      "#FFC0CB",
	"Brown":     "#A52A2A",
	"Gold":      "#FFD700",
	"Beige":     "#F5F5DC",
	"Turquoise": "#40E0D0",
	"Lavender":  "#E6E6FA",
	"Chocolate": "#D2691E",
	"Coral":     "#FF7F50",
}

func main() {
	// Check if a file path is provided as a command-line argument
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <svg-file-path>")
		os.Exit(1)
	}

	// Input SVG file path from the command line
	filePath := os.Args[1]

	// Open the SVG file for reading
	data, err := os.ReadFile(filePath)
	if err != nil {
		panic(fmt.Sprintf("Error reading file: %v", err))
	}

	// Regex to find attributes like fill and stroke with their color values
	colorRegex := regexp.MustCompile(`\b(fill|stroke)=["'](#?[0-9a-fA-F]{3,6}|[a-zA-Z]+)["']`)

	// Map to track color usage by attribute and color value
	colorUsage := make(map[string]map[string]int)

	// Find all matches for color attributes in the SVG content
	matches := colorRegex.FindAllStringSubmatch(string(data), -1)

	// Process matches to count occurrences of each color per attribute
	for _, match := range matches {
		attribute := match[1] // Extract the attribute (e.g., fill or stroke)
		color := match[2]     // Extract the color value
		if _, exists := colorUsage[attribute]; !exists {
			colorUsage[attribute] = make(map[string]int) // Initialize map for the attribute
		}
		colorUsage[attribute][color]++ // Increment the count for the color
	}

	// Print the analysis of color usage
	for attr, colors := range colorUsage {
		fmt.Printf("Attribute: %s\n", attr)
		for color, count := range colors {
			fmt.Printf("  Color: %s, Count: %d\n", color, count)
			// Find and print the closest human-readable color names
			printClosestHumanReadableColors(color)
		}
	}
}

func printClosestHumanReadableColors(color string) {
	fmt.Println("  Closest Matches:")

	// Parse the input color into a colorful.Color object
	inputColor, err := colorful.Hex(color)
	if err != nil {
		fmt.Printf("    Error parsing color %s: %v\n", color, err)
		return
	}

	// Calculate distances to predefined colors and store matches
	matches := []ColorMatch{}
	for name, hex := range humanReadableColors {
		referenceColor, _ := colorful.Hex(hex)             // Parse the predefined color
		distance := inputColor.DistanceLab(referenceColor) // Calculate perceptual distance
		matches = append(matches, ColorMatch{Name: name, ColorHex: hex, Distance: distance})
	}

	// Sort matches by distance (smallest to largest)
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Distance < matches[j].Distance
	})

	// Print the top 24 closest matches
	for i, match := range matches {
		if i >= 24 {
			break
		}
		fmt.Printf("    %s (%s), Distance: %.4f\n", match.Name, match.ColorHex, match.Distance)
	}
}
