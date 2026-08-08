package graph

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/itchyny/gojq"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2layouts/d2elklayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	d2log "oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

// GenerateSVG renders a query's structure as an SVG document.
func GenerateSVG(query *gojq.Query) (string, error) {
	svg, err := renderSVG(RenderD2(query))
	if err != nil {
		return "", err
	}
	return string(svg), nil
}

// GenerateGraph creates a D2 diagram representing the flow of a jq query
func GenerateGraph(query *gojq.Query, outputPath string) error {
	outputPath, err := filepath.Abs(outputPath)
	if err != nil {
		return fmt.Errorf("failed to resolve output path: %w", err)
	}

	script := RenderD2(query)

	switch ext := strings.ToLower(filepath.Ext(outputPath)); ext {
	case ".d2":
		return os.WriteFile(outputPath, []byte(script), 0644)

	case ".svg":
		svg, err := renderSVG(script)
		if err != nil {
			// Keep the script alongside the failure so the diagram can be
			// debugged without re-running the query.
			scriptPath := strings.TrimSuffix(outputPath, ext) + ".d2"
			os.WriteFile(scriptPath, []byte(script), 0644)
			return fmt.Errorf("%w\nD2 script saved to: %s", err, scriptPath)
		}
		return os.WriteFile(outputPath, svg, 0644)

	default:
		return fmt.Errorf("unsupported output format: %s (supported formats: .d2, .svg)", ext)
	}
}

// renderSVG compiles a D2 script and renders it.
func renderSVG(script string) ([]byte, error) {
	ctx := d2log.With(context.Background(),
		slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))

	ruler, err := textmeasure.NewRuler()
	if err != nil {
		return nil, fmt.Errorf("failed to create text ruler: %w", err)
	}

	layout := "dagre"
	diagram, _, err := d2lib.Compile(ctx, script, &d2lib.CompileOptions{
		Layout: &layout,
		Ruler:  ruler,
		LayoutResolver: func(engine string) (d2graph.LayoutGraph, error) {
			switch engine {
			case "elk":
				return d2elklayout.DefaultLayout, nil
			case "dagre":
				return d2dagrelayout.DefaultLayout, nil
			}
			return nil, fmt.Errorf("unknown layout engine: %s", engine)
		},
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to compile D2 diagram: %w", err)
	}

	pad := int64(d2svg.DEFAULT_PADDING)
	themeID := int64(200) // dark-mauve
	svg, err := d2svg.Render(diagram, &d2svg.RenderOpts{Pad: &pad, ThemeID: &themeID})
	if err != nil {
		return nil, fmt.Errorf("failed to render D2 diagram to SVG: %w", err)
	}
	return svg, nil
}
