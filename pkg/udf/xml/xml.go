package xml

import (
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterXMLParse registers the xml_parse function with gojq
func RegisterXMLParse() gojq.CompilerOption {
	return gojq.WithFunction("xml_parse", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("xml_parse: %v", err), nil)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var input string
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("xml_parse: file argument requires string path, got %T", inputVal), nil)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("xml_parse: %v", err), nil)
			}

			input = string(fileData)
			filePath = absPath
			fileSize = size
		} else {
			switch val := inputVal.(type) {
			case string:
				input = val
			case []byte:
				input = string(val)
			default:
				if str, ok := val.(fmt.Stringer); ok {
					input = str.String()
				} else {
					return common.MakeUDFErrorResult(fmt.Errorf("xml_parse: argument must be a string, got %T", val), nil)
				}
			}
		}

		// Parse XML - use a simple map structure
		// Note: Full XML parsing is complex, so we'll use a simplified approach
		var result map[string]any
		decoder := xml.NewDecoder(strings.NewReader(input))
		
		// For simplicity, we'll parse into a generic structure
		// This is a basic implementation - full XML parsing would require more complex handling
		var xmlData struct {
			XMLName xml.Name
			Content []byte `xml:",innerxml"`
			Attrs   []xml.Attr `xml:",any,attr"`
		}
		
		if err := decoder.Decode(&xmlData); err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("xml_parse: failed to parse XML: %v", err), nil)
		}

		// Build result object
		result = make(map[string]any)
		result["_tag"] = xmlData.XMLName.Local
		if len(xmlData.Attrs) > 0 {
			attrs := make(map[string]any)
			for _, attr := range xmlData.Attrs {
				attrs[attr.Name.Local] = attr.Value
			}
			result["_attrs"] = attrs
		}
		if len(xmlData.Content) > 0 {
			result["_content"] = string(xmlData.Content)
		}

		meta := map[string]any{
			"operation": "xml_parse",
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
		} else {
			meta["input_length"] = len(input)
		}

		// Return parsed object directly
		return result
	})
}

// RegisterXMLStringify registers the xml_stringify function with gojq
func RegisterXMLStringify() gojq.CompilerOption {
	return gojq.WithFunction("xml_stringify", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("xml_stringify: %v", err), nil)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		// Convert object to XML
		// This is a simplified implementation
		var result string
		
		switch val := inputVal.(type) {
		case map[string]any:
			// Try to extract tag name and content
			tagName := "root"
			if tag, ok := val["_tag"].(string); ok {
				tagName = tag
			}
			
			var attrs []string
			if attrsMap, ok := val["_attrs"].(map[string]any); ok {
				for k, v := range attrsMap {
					attrs = append(attrs, fmt.Sprintf(`%s="%s"`, k, fmt.Sprintf("%v", v)))
				}
			}
			
			content := ""
			if c, ok := val["_content"].(string); ok {
				content = c
			} else {
				// Try to marshal the whole object
				xmlBytes, err := xml.MarshalIndent(val, "", "  ")
				if err == nil {
					content = string(xmlBytes)
				}
			}
			
			attrStr := ""
			if len(attrs) > 0 {
				attrStr = " " + strings.Join(attrs, " ")
			}
			
			if content != "" {
				result = fmt.Sprintf("<%s%s>%s</%s>", tagName, attrStr, content, tagName)
			} else {
				result = fmt.Sprintf("<%s%s/>", tagName, attrStr)
			}
		default:
			// For non-map types, try to marshal directly
			xmlBytes, err := xml.MarshalIndent(val, "", "  ")
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("xml_stringify: failed to marshal: %v", err), nil)
			}
			result = string(xmlBytes)
		}

		meta := map[string]any{
			"operation": "xml_stringify",
			"output_length": len(result),
		}

		if isFile {
			filePathStr, ok := inputVal.(string)
			if ok {
				_, absPath, size, err := common.ReadFileFromPath(filePathStr)
				if err == nil {
					meta["file_path"] = absPath
					meta["file_size"] = int(size)
				}
			}
		}

  return common.MakeUDFSuccessResult(result, meta)
	})
}

