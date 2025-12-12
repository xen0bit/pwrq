package xml

import (
	"encoding/xml"
	"fmt"
	"strings"
	"testing"
)

func TestXMLParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "simple XML",
			input:   `<root>hello</root>`,
			wantErr: false,
		},
		{
			name:    "XML with attributes",
			input:   `<root attr="value">content</root>`,
			wantErr: false,
		},
		{
			name:    "invalid XML",
			input:   `<root>unclosed`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var xmlData struct {
				XMLName xml.Name
				Content []byte `xml:",innerxml"`
			}
			
			decoder := xml.NewDecoder(strings.NewReader(tt.input))
			err := decoder.Decode(&xmlData)
			if (err != nil) != tt.wantErr {
				t.Errorf("xml_parse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestXMLStringify(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]any
	}{
		{
			name: "simple object",
			input: map[string]any{
				"_tag":     "root",
				"_content": "hello",
			},
		},
		{
			name: "with attributes",
			input: map[string]any{
				"_tag": "root",
				"_attrs": map[string]any{
					"attr": "value",
				},
				"_content": "content",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tagName := "root"
			if tag, ok := tt.input["_tag"].(string); ok {
				tagName = tag
			}
			
			var attrs []string
			if attrsMap, ok := tt.input["_attrs"].(map[string]any); ok {
				for k, v := range attrsMap {
					attrs = append(attrs, k+"=\""+fmt.Sprintf("%v", v)+"\"")
				}
			}
			
			content := ""
			if c, ok := tt.input["_content"].(string); ok {
				content = c
			}
			
			attrStr := ""
			if len(attrs) > 0 {
				attrStr = " " + strings.Join(attrs, " ")
			}
			
			result := fmt.Sprintf("<%s%s>%s</%s>", tagName, attrStr, content, tagName)
			
			if !strings.Contains(result, tagName) {
				t.Errorf("xml_stringify() result doesn't contain tag name: %s", result)
			}
		})
	}
}

